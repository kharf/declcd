package project

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/kharf/declcd/pkg/vcs"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler clones, pulls and loads a GitOps Git repository containing the desired cluster state,
// then decrypts secrets, translates cue definitions to either Kubernetes unstructurd objects or Helm Releases and applies/installs them on a Kubernetes cluster.
// Every run stores objects in the inventory and collects dangling objects.
type Reconciler struct {
	Log               logr.Logger
	Client            client.Client
	ProjectManager    Manager
	RepositoryManager vcs.RepositoryManager
	ComponentBuilder  component.Builder
	ChartReconciler   helm.ChartReconciler
	InventoryManager  *inventory.Manager
	GarbageCollector  garbage.Collector
	Decrypter         secret.Decrypter
	WorkerPoolSize    int
}

type ReconcileResult struct {
	Suspended bool
}

// Reconcile clones, pulls and loads a GitOps Git repository containing the desired cluster state,
// then decrypts secrets, translates cue definitions to either Kubernetes unstructurd objects or Helm Releases and applies/installs them on a Kubernetes cluster.
// It stores objects in the inventory and collects dangling objects.
func (reconciler Reconciler) Reconcile(
	ctx context.Context,
	gProject gitopsv1.GitOpsProject,
) (*ReconcileResult, error) {
	log := reconciler.Log
	if *gProject.Spec.Suspend {
		return &ReconcileResult{Suspended: true}, nil
	}
	reconcileResult := &ReconcileResult{Suspended: false}
	repositoryUID := string(gProject.GetUID())
	repositoryDir := filepath.Join(os.TempDir(), "declcd", repositoryUID)
	repository, err := reconciler.RepositoryManager.Load(
		ctx,
		vcs.WithUrl(gProject.Spec.URL),
		vcs.WithTarget(repositoryDir),
	)
	if err != nil {
		log.Error(
			err,
			"Unable to load gitops project repository",
			"project",
			gProject.GetName(),
			"repository",
			gProject.Spec.URL,
		)
		return reconcileResult, err
	}
	if err := repository.Pull(); err != nil {
		log.Error(
			err,
			"Unable to pull gitops project repository",
			"project",
			gProject.GetName(),
			"repository",
			gProject.Spec.URL,
		)
		return reconcileResult, err
	}
	repositoryDir, err = reconciler.Decrypter.Decrypt(ctx, repositoryDir)
	if err != nil {
		log.Error(err, "Unable to decrypt secrets", "project", gProject.GetName())
		return reconcileResult, err
	}
	dependencyGraph, err := reconciler.ProjectManager.Load(repositoryDir)
	if err != nil {
		log.Error(err, "Unable to load declcd project", "project", gProject.GetName())
		return reconcileResult, err
	}
	componentNodes, err := dependencyGraph.TopologicalSort()
	if err != nil {
		log.Error(err, "Unable to resolve dependencies", "project", gProject.GetName())
		return reconcileResult, err
	}
	if err := reconciler.GarbageCollector.Collect(ctx, *dependencyGraph); err != nil {
		return reconcileResult, err
	}
	if err := reconciler.reconcileComponents(ctx, componentNodes, repositoryDir); err != nil {
		log.Error(err, "Unable to reconcile components", "project", gProject.GetName())
		return reconcileResult, err
	}
	return reconcileResult, nil
}

func (reconciler Reconciler) reconcileComponents(
	ctx context.Context,
	componentNodes []component.Node,
	repositoryDir string,
) error {
	componentBuilder := reconciler.ComponentBuilder
	eg := errgroup.Group{}
	eg.SetLimit(reconciler.WorkerPoolSize)
	for _, node := range componentNodes {
		eg.Go(func() error {
			return reconciler.newMethod(
				ctx,
				componentBuilder,
				repositoryDir,
				node,
			)
		})
	}
	return eg.Wait()
}

func (reconciler Reconciler) newMethod(
	ctx context.Context,
	componentBuilder component.Builder,
	repositoryDir string,
	node component.Node,
) error {
	componentInstance, err := componentBuilder.Build(
		component.WithProjectRoot(repositoryDir),
		component.WithComponentPath(node.Path()),
	)
	if err != nil {
		return err
	}
	component := *componentInstance
	if err := reconciler.reconcileManifests(ctx, component); err != nil {
		return err
	}
	if err := reconciler.reconcileHelmReleases(ctx, component); err != nil {
		return err
	}
	return nil
}

func (reconciler Reconciler) reconcileManifests(
	ctx context.Context,
	componentInstance component.Instance,
) error {
	for _, manifest := range componentInstance.Manifests {
		if err := reconciler.createOrUpdate(ctx, &manifest); err != nil {
			return err
		}
		invManifest := inventory.NewManifestItem(
			v1.TypeMeta{
				Kind:       manifest.GetKind(),
				APIVersion: manifest.GetAPIVersion(),
			},
			componentInstance.ID,
			manifest.GetName(),
			manifest.GetNamespace(),
		)
		if err := reconciler.InventoryManager.StoreItem(invManifest, nil); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler Reconciler) reconcileHelmReleases(
	ctx context.Context,
	componentInstance component.Instance,
) error {
	for _, release := range componentInstance.HelmReleases {
		if _, err := reconciler.ChartReconciler.Reconcile(
			ctx,
			componentInstance.ID,
			release,
		); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler Reconciler) createOrUpdate(
	ctx context.Context,
	manifest *unstructured.Unstructured,
) error {
	client := reconciler.Client
	log := reconciler.Log
	log.Info(
		"Applying manifest",
		"namespace",
		manifest.GetNamespace(),
		"name",
		manifest.GetName(),
		"kind",
		manifest.GetKind(),
	)
	if err := client.Create(ctx, manifest); err != nil {
		if errors.IsAlreadyExists(err) {
			currentManifest := unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       manifest.GetKind(),
				"apiVersion": manifest.GetAPIVersion(),
			}}
			if err := client.Get(ctx, types.NamespacedName{Namespace: manifest.GetNamespace(), Name: manifest.GetName()}, &currentManifest); err != nil {
				return err
			}
			manifest.SetResourceVersion(currentManifest.GetResourceVersion())
			return client.Update(ctx, manifest)
		}
		return err
	}
	return nil
}
