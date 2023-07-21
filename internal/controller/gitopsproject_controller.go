/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/project"
)

// GitOpsProjectReconciler reconciles a GitOpsProject object
type GitOpsProjectReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	CueContext     *cue.Context
	ProjectManager project.ProjectManager
	repository     *project.Repository
}

//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (reconciler *GitOpsProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciliation triggered")

	var gProject gitopsv1.GitOpsProject
	if err := reconciler.Get(ctx, req.NamespacedName, &gProject); err != nil {
		log.Error(err, "unable to fetch the GitOpsProject resource from the cluster")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	requeueResult := ctrl.Result{RequeueAfter: time.Duration(gProject.Spec.PullIntervalSeconds) * time.Second}

	repositoryUID := string(gProject.GetUID())
	repositoryDir := filepath.Join(reconciler.ProjectManager.FS.Root, repositoryUID)
	if reconciler.repository == nil {
		repoManager := project.NewRepositoryManager()
		if err := os.RemoveAll(repositoryDir); err != nil {
			return requeueResult, err
		}
		repository, err := repoManager.Clone(project.WithUrl(gProject.Spec.URL), project.WithTarget(repositoryDir))
		if err != nil {
			log.Error(err, "unable to clone gitops project repository", "repository", gProject.Spec.URL)
			return requeueResult, err
		}
		reconciler.repository = repository
	}

	if err := reconciler.repository.Pull(); err != nil {
		log.Error(err, "unable to pull gitops project repository")
		return requeueResult, err
	}

	mainComponents, err := reconciler.ProjectManager.Load(repositoryUID)
	if err != nil {
		log.Error(err, "unable to load decl project")
		return requeueResult, err
	}

	if err := reconciler.reconcileComponents(ctx, mainComponents, repositoryDir); err != nil {
		log.Error(err, "unable to reconcile components")
		return requeueResult, err
	}

	pullTime := v1.Now()
	gProject.Status.LastPullTime = &pullTime
	if err := reconciler.Status().Update(ctx, &gProject); err != nil {
		log.Error(err, "unable to update GitOpsProject status")
		return requeueResult, err
	}

	log.Info("reconciliation finished")

	return requeueResult, nil
}

func (reconciler *GitOpsProjectReconciler) reconcileComponents(ctx context.Context, mainComponents []project.MainDeclarativeComponent, repositoryDir string) error {
	componentBuilder := project.NewComponentBuilder(reconciler.CueContext)
	for _, mainComponent := range mainComponents {
		for _, subComponent := range mainComponent.SubComponents {
			component, err := componentBuilder.Build(project.WithProjectRoot(repositoryDir), project.WithComponentPath(subComponent.Path))
			if err != nil {
				return err
			}

			if err := reconciler.reconcileManifests(ctx, component.Manifests); err != nil {
				return nil
			}
		}
	}
	return nil
}

func (reconciler *GitOpsProjectReconciler) reconcileManifests(ctx context.Context, manifests []unstructured.Unstructured) error {
	for _, manifest := range manifests {
		if err := reconciler.createOrUpdate(ctx, &manifest); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler *GitOpsProjectReconciler) createOrUpdate(ctx context.Context, manifest *unstructured.Unstructured) error {
	if err := reconciler.Create(ctx, manifest); err != nil {
		if errors.IsAlreadyExists(err) {
			return reconciler.Update(ctx, manifest)
		}
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (reconciler *GitOpsProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitopsv1.GitOpsProject{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(reconciler)
}
