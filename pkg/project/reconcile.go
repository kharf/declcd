package project

import (
	"context"
	"path/filepath"

	"cuelang.org/go/cue"
	"github.com/go-logr/logr"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/helm"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	Client            client.Client
	ProjectManager    ProjectManager
	RepositoryManager RepositoryManager
	CueContext        *cue.Context
	ChartReconciler   helm.ChartReconciler
	Log               logr.Logger
}

type ReconcileResult struct {
	Suspended bool
}

func (reconciler Reconciler) Reconcile(ctx context.Context, gProject gitopsv1.GitOpsProject) (*ReconcileResult, error) {
	log := reconciler.Log
	if *gProject.Spec.Suspend {
		return &ReconcileResult{Suspended: true}, nil
	}

	reconcileResult := &ReconcileResult{Suspended: false}
	repositoryUID := string(gProject.GetUID())
	repositoryDir := filepath.Join(reconciler.ProjectManager.FS.Root, repositoryUID)
	repository, err := reconciler.RepositoryManager.Load(WithUrl(gProject.Spec.URL), WithTarget(repositoryDir))
	if err != nil {
		log.Error(err, "unable to load gitops project repository", "repository", gProject.Spec.URL)
		return reconcileResult, err
	}

	if err := repository.Pull(); err != nil {
		log.Error(err, "unable to pull gitops project repository", "repository", gProject.Spec.URL)
		return reconcileResult, err
	}

	mainComponents, err := reconciler.ProjectManager.Load(repositoryUID)
	if err != nil {
		log.Error(err, "unable to load decl project")
		return reconcileResult, err
	}

	if err := reconciler.reconcileComponents(ctx, mainComponents, repositoryDir); err != nil {
		log.Error(err, "unable to reconcile components")
		return reconcileResult, err
	}

	return reconcileResult, nil
}

func (reconciler Reconciler) reconcileComponents(ctx context.Context, mainComponents []MainDeclarativeComponent, repositoryDir string) error {
	componentBuilder := NewComponentBuilder(reconciler.CueContext)
	for _, mainComponent := range mainComponents {
		if err := reconciler.reconcileSubComponents(ctx, mainComponent.SubComponents, repositoryDir, componentBuilder); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler Reconciler) reconcileSubComponents(ctx context.Context, subComponents []*SubDeclarativeComponent, repositoryDir string, componentBuilder ComponentBuilder) error {
	for _, subComponent := range subComponents {
		component, err := componentBuilder.Build(WithProjectRoot(repositoryDir), WithComponentPath(subComponent.Path))
		if err != nil {
			return err
		}

		if err := reconciler.reconcileManifests(ctx, component.Manifests); err != nil {
			return err
		}

		if err := reconciler.reconcileHelmReleases(ctx, component.HelmReleases); err != nil {
			return err
		}

		if err := reconciler.reconcileSubComponents(ctx, subComponent.SubComponents, repositoryDir, componentBuilder); err != nil {
			return err
		}

	}
	return nil
}

func (reconciler Reconciler) reconcileManifests(ctx context.Context, manifests []unstructured.Unstructured) error {
	for _, manifest := range manifests {
		if err := reconciler.createOrUpdate(ctx, &manifest); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler Reconciler) reconcileHelmReleases(ctx context.Context, releases []helm.Release) error {
	for _, release := range releases {
		if _, err := reconciler.ChartReconciler.Reconcile(
			ctx,
			release.Chart,
			release.Values,
			helm.ReleaseName(release.Name),
			helm.Namespace(release.Namespace),
		); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler Reconciler) createOrUpdate(ctx context.Context, manifest *unstructured.Unstructured) error {
	if err := reconciler.Client.Create(ctx, manifest); err != nil {
		if errors.IsAlreadyExists(err) {
			return reconciler.Client.Update(ctx, manifest)
		}
		return err
	}
	return nil
}
