package project

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/kharf/declcd/pkg/vcs"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler clones, pulls and loads a GitOps Git repository containing the desired cluster state,
// then decrypts secrets, translates cue definitions to either Kubernetes unstructurd objects or Helm Releases and applies/installs them on a Kubernetes cluster.
// Every run stores objects in the inventory and collects dangling objects.
type Reconciler struct {
	Log               logr.Logger
	Client            client.Client
	DynamicClient     kube.Client[unstructured.Unstructured]
	ProjectManager    Manager
	RepositoryManager vcs.RepositoryManager
	ComponentBuilder  component.Builder
	ChartReconciler   helm.ChartReconciler
	InventoryManager  *inventory.Manager
	GarbageCollector  garbage.Collector
	Decrypter         secret.Decrypter
	FieldManager      string
	WorkerPoolSize    int
}

type ReconcileResult struct {
	Suspended  bool
	CommitHash string
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
		return nil, err
	}
	commitHash, err := repository.Pull()
	if err != nil {
		log.Error(
			err,
			"Unable to pull gitops project repository",
			"project",
			gProject.GetName(),
			"repository",
			gProject.Spec.URL,
		)
		return nil, err
	}
	repositoryDir, err = reconciler.Decrypter.Decrypt(ctx, repositoryDir)
	if err != nil {
		log.Error(err, "Unable to decrypt secrets", "project", gProject.GetName())
		return nil, err
	}
	dependencyGraph, err := reconciler.ProjectManager.Load(repositoryDir)
	if err != nil {
		log.Error(err, "Unable to load declcd project", "project", gProject.GetName())
		return nil, err
	}
	componentInstances, err := dependencyGraph.TopologicalSort()
	if err != nil {
		log.Error(err, "Unable to resolve dependencies", "project", gProject.GetName())
		return nil, err
	}
	if err := reconciler.GarbageCollector.Collect(ctx, dependencyGraph); err != nil {
		return nil, err
	}
	if err := reconciler.reconcileComponents(ctx, componentInstances, repositoryDir); err != nil {
		log.Error(err, "Unable to reconcile components", "project", gProject.GetName())
		return nil, err
	}

	return &ReconcileResult{
		Suspended:  false,
		CommitHash: commitHash,
	}, nil
}

func (reconciler Reconciler) reconcileComponents(
	ctx context.Context,
	componentInstances []component.Instance,
	repositoryDir string,
) error {
	eg := errgroup.Group{}
	eg.SetLimit(reconciler.WorkerPoolSize)
	for _, instance := range componentInstances {
		// TODO: implement SCC decomposition for better concurrency/parallelism
		if len(instance.GetDependencies()) == 0 {
			eg.Go(func() error {
				return reconciler.reconcileComponent(
					ctx,
					repositoryDir,
					instance,
				)
			})
		} else {
			if err := eg.Wait(); err != nil {
				return err
			}
			if err := reconciler.reconcileComponent(
				ctx,
				repositoryDir,
				instance,
			); err != nil {
				return err
			}
		}
	}
	return eg.Wait()
}

func (reconciler Reconciler) reconcileComponent(
	ctx context.Context,
	repositoryDir string,
	componentInstance component.Instance,
) error {
	switch componentInstance := componentInstance.(type) {
	case *component.Manifest:
		reconciler.Log.Info(
			"Applying manifest",
			"namespace",
			componentInstance.Content.GetNamespace(),
			"name",
			componentInstance.Content.GetName(),
			"kind",
			componentInstance.Content.GetKind(),
		)
		if err := reconciler.DynamicClient.Apply(ctx, &componentInstance.Content, reconciler.FieldManager, kube.Force(true)); err != nil {
			return err
		}
		invManifest := &inventory.ManifestItem{
			ID: componentInstance.ID,
			TypeMeta: v1.TypeMeta{
				Kind:       componentInstance.Content.GetKind(),
				APIVersion: componentInstance.Content.GetAPIVersion(),
			},
			Name:      componentInstance.Content.GetName(),
			Namespace: componentInstance.Content.GetNamespace(),
		}
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(componentInstance.Content.Object); err != nil {
			return err
		}
		if err := reconciler.InventoryManager.StoreItem(invManifest, buf); err != nil {
			return err
		}
	case *component.HelmRelease:
		if _, err := reconciler.ChartReconciler.Reconcile(
			ctx,
			componentInstance.Content,
			componentInstance.ID,
		); err != nil {
			return err
		}
	}
	return nil
}
