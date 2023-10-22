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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (reconciler *GitOpsProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciliation triggered")
	triggerTime := v1.Now()

	var gProject gitopsv1.GitOpsProject
	if err := reconciler.Reconciler.Client.Get(ctx, req.NamespacedName, &gProject); err != nil {
		log.Error(err, "unable to fetch the GitOpsProject resource from the cluster")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	requeueResult := ctrl.Result{RequeueAfter: time.Duration(gProject.Spec.PullIntervalSeconds) * time.Second}

	gProject.Status.Conditions = make([]v1.Condition, 0, 2)
	if err := reconciler.updateCondition(ctx, &gProject, v1.Condition{
		Type:               "Running",
		Reason:             "Interval",
		Message:            "Reconciling",
		Status:             "True",
		LastTransitionTime: triggerTime,
	}); err != nil {
		log.Error(err, "unable to update GitOpsProject status condition to 'Running'")
		return requeueResult, nil
	}

	_, err := reconciler.Reconciler.Reconcile(ctx, gProject)
	if err != nil {
		log.Error(err, "reconciliation failed")
		return requeueResult, nil
	}

	reconciledTime := v1.Now()
	if err := reconciler.updateCondition(ctx, &gProject, v1.Condition{
		Type:               "Finished",
		Reason:             "Success",
		Message:            "Reconciled",
		Status:             "True",
		LastTransitionTime: reconciledTime,
	}); err != nil {
		log.Error(err, "unable to update GitOpsProject status")
		return requeueResult, nil
	}

	log.Info("reconciliation finished")

	return requeueResult, nil
}

func (reconciler *GitOpsProjectReconciler) updateCondition(ctx context.Context, gProject *gitopsv1.GitOpsProject, condition v1.Condition) error {
	gProject.Status.Conditions = append(gProject.Status.Conditions, condition)
	if err := reconciler.Reconciler.Client.Status().Update(ctx, gProject); err != nil {
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
