// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/vcs"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/rest"
)

// Reconciler clones, pulls and loads a GitOps Git repository containing the desired cluster state,
// translates cue definitions to either Kubernetes unstructurd objects or Helm Releases and applies/installs them on a Kubernetes cluster.
// Every run stores objects in the inventory and collects dangling objects.
type Reconciler struct {
	Log logr.Logger

	KubeConfig *rest.Config

	// Manager loads a declcd project and resolves the component dependency graph.
	ProjectManager Manager

	// RepositoryManager clones a remote vcs repository to a local path.
	RepositoryManager vcs.RepositoryManager

	// Managers identify distinct workflows that are modifying the object (especially useful on conflicts!),
	FieldManager string

	// Defines the concurrency level of Declcd operations.
	WorkerPoolSize int

	// InsecureSkipVerify controls whether Helm clients verify server
	// certificate chains and host names.
	InsecureSkipTLSverify bool

	// Force http for Helm registries.
	PlainHTTP bool
}

// ReconcileResult reports the outcome and metadata of a reconciliation.
type ReconcileResult struct {
	// Reports whether the GitOpsProject was flagged as suspended.
	Suspended bool

	// The hash of the reconciled Git Commit.
	CommitHash string
}

// Reconcile clones, pulls and loads a GitOps Git repository containing the desired cluster state,
// translates cue definitions to either Kubernetes unstructurd objects or Helm Releases and applies/installs them on a Kubernetes cluster.
// It stores objects in the inventory and collects dangling objects.
func (reconciler *Reconciler) Reconcile(
	ctx context.Context,
	gProject gitops.GitOpsProject,
) (*ReconcileResult, error) {
	if *gProject.Spec.Suspend {
		return &ReconcileResult{Suspended: true}, nil
	}
	log := reconciler.Log

	cfg := reconciler.KubeConfig
	if gProject.Spec.ServiceAccountName != "" {
		cfg.Impersonate = rest.ImpersonationConfig{
			UserName: fmt.Sprintf(
				"system:serviceaccount:%s:%s",
				gProject.Namespace,
				gProject.Spec.ServiceAccountName,
			),
		}
	}

	log = log.WithValues(
		"project",
		gProject.GetName(),
		"repository",
		gProject.Spec.URL,
		"impersonated",
		gProject.Spec.ServiceAccountName,
	)

	kubeDynamicClient, err := kube.NewExtendedDynamicClient(cfg)
	if err != nil {
		log.Error(
			err,
			"Unable to create Kubernetes Client",
		)
		return nil, err
	}

	projectUID := string(gProject.GetUID())
	repositoryDir := filepath.Join(os.TempDir(), "declcd", projectUID)

	inventoryInstance := &inventory.Instance{
		// /inventory is mounted as volume.
		Path: filepath.Join("/inventory", projectUID),
	}

	chartReconciler := helm.ChartReconciler{
		KubeConfig:            cfg,
		Client:                kubeDynamicClient,
		FieldManager:          reconciler.FieldManager,
		InventoryInstance:     inventoryInstance,
		InsecureSkipTLSverify: reconciler.InsecureSkipTLSverify,
		PlainHTTP:             reconciler.PlainHTTP,
		Log:                   log,
	}

	garbageCollector := garbage.Collector{
		Log:               log,
		Client:            kubeDynamicClient.DynamicClient(),
		ChartReconciler:   chartReconciler,
		InventoryInstance: inventoryInstance,
		WorkerPoolSize:    reconciler.WorkerPoolSize,
	}

	repository, err := reconciler.RepositoryManager.Load(
		ctx,
		gProject.Spec.URL,
		gProject.Spec.Branch,
		repositoryDir,
		gProject.Name,
	)
	if err != nil {
		log.Error(
			err,
			"Unable to load gitops project repository",
		)
		return nil, err
	}

	commitHash, err := repository.Pull()
	if err != nil {
		log.Error(
			err,
			"Unable to pull gitops project repository",
		)
		return nil, err
	}

	instances, err := reconciler.ProjectManager.Load(repositoryDir)
	if err != nil {
		log.Error(
			err,
			"Unable to load declcd project",
		)
		return nil, err
	}

	dag := component.NewDependencyGraph()
	if err := dag.Insert(instances.Components...); err != nil {
		log.Error(
			err,
			"Unable to set up dependency graph",
		)
		return nil, err
	}

	components, err := dag.TopologicalSort()
	if err != nil {
		log.Error(
			err,
			"Unable to resolve dependencies",
		)
		return nil, err
	}

	instances.Components = components

	if err := garbageCollector.Collect(ctx, &dag); err != nil {
		return nil, err
	}

	componentReconciler := component.Reconciler{
		Log:               log,
		DynamicClient:     kubeDynamicClient,
		ChartReconciler:   chartReconciler,
		InventoryInstance: inventoryInstance,
		FieldManager:      reconciler.FieldManager,
	}

	if err := reconciler.reconcileComponents(ctx, componentReconciler, instances); err != nil {
		log.Error(
			err,
			"Unable to reconcile components",
		)
		return nil, err
	}

	return &ReconcileResult{
		Suspended:  false,
		CommitHash: commitHash,
	}, nil
}

func (reconciler *Reconciler) reconcileComponents(
	ctx context.Context,
	componentReconciler component.Reconciler,
	componentInstances *component.Instances,
) error {
	eg := errgroup.Group{}
	eg.SetLimit(reconciler.WorkerPoolSize)
	for _, instance := range componentInstances.Components {
		// TODO: implement SCC decomposition for better concurrency/parallelism
		if len(instance.Dependencies) == 0 {
			eg.Go(func() error {
				return componentReconciler.Reconcile(
					ctx,
					componentInstances.All[instance.Content],
				)
			})
		} else {
			if err := eg.Wait(); err != nil {
				return err
			}
			if err := componentReconciler.Reconcile(
				ctx,
				componentInstances.All[instance.Content],
			); err != nil {
				return err
			}
		}
	}
	return eg.Wait()
}
