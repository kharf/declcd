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
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/project"
)

// GitOpsProjectReconciler reconciles a GitOpsProject object
type GitOpsProjectReconciler struct {
	Reconciler project.Reconciler
}

//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gitops.declcd.io,resources=gitopsprojects/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (gReconciler *GitOpsProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciliation triggered")

	var gProject gitopsv1.GitOpsProject
	if err := gReconciler.Reconciler.Client.Get(ctx, req.NamespacedName, &gProject); err != nil {
		log.Error(err, "unable to fetch the GitOpsProject resource from the cluster")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	requeueResult := ctrl.Result{RequeueAfter: time.Duration(gProject.Spec.PullIntervalSeconds) * time.Second}

	_, err := gReconciler.Reconciler.Reconcile(ctx, gProject)
	if err != nil {
		log.Info("reconciliation failed")
		return requeueResult, err
	}

	pullTime := v1.Now()
	gProject.Status.LastPullTime = &pullTime
	if err := gReconciler.Reconciler.Client.Status().Update(ctx, &gProject); err != nil {
		log.Error(err, "unable to update GitOpsProject status")
		return requeueResult, err
	}
	log.Info("reconciliation finished")

	return requeueResult, nil
}

// SetupWithManager sets up the controller with the Manager.
func (reconciler *GitOpsProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitopsv1.GitOpsProject{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(reconciler)
}
