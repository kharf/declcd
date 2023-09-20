package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestReconciler_Reconcile(t *testing.T) {
	env := projecttest.StartProjectEnv(t)
	defer env.Stop()
	cueCtx := cuecontext.New()
	chartReconciler := helm.ChartReconciler{
		Cfg: *env.HelmConfig,
		Log: env.Log,
	}

	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		CueContext:        cueCtx,
		RepositoryManager: env.RepositoryManager,
		ProjectManager:    env.ProjectManager,
		ChartReconciler:   chartReconciler,
		InventoryManager:  env.InventoryManager,
		GarbageCollector:  env.GarbageCollector,
		Log:               env.Log,
	}

	suspend := false
	gProject := gitopsv1.GitOpsProject{
		TypeMeta: v1.TypeMeta{
			APIVersion: "gitops.declcd.io/v1",
			Kind:       "GitOpsProject",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "reconcile-test",
			Namespace: "default",
			UID:       "reconcile-test",
		},
		Spec: gitopsv1.GitOpsProjectSpec{
			URL:                 env.TestProject,
			PullIntervalSeconds: 5,
			Suspend:             &suspend,
		},
	}
	result, err := reconciler.Reconcile(env.Ctx, gProject)
	assert.NilError(t, err)
	assert.Equal(t, result.Suspended, false)

	ctx := context.Background()
	var mysubcomponent appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: "mynamespace"}, &mysubcomponent)
	assert.NilError(t, err)
	assert.Equal(t, mysubcomponent.Name, "mysubcomponent")
	var test appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: "mynamespace"}, &test)
	assert.NilError(t, err)
	assert.Equal(t, test.Name, "test")

	inventoryStorage, err := reconciler.InventoryManager.Load()
	assert.NilError(t, err)
	assert.Assert(t, len(inventoryStorage.Manifests) == 2)
	subComponentDeploymentManifest := inventory.Manifest{
		TypeMeta: v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Name:      mysubcomponent.Name,
		Namespace: mysubcomponent.Namespace,
	}
	assert.Assert(t, inventoryStorage.Has(subComponentDeploymentManifest))
	mynamespace := inventory.Manifest{
		TypeMeta: v1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		Name: mysubcomponent.Namespace,
	}
	assert.Assert(t, inventoryStorage.Has(mynamespace))

	err = os.Remove(filepath.Join(env.TestProject, "infra", "prometheus", "subcomponent", "component.cue"))
	assert.NilError(t, err)
	err = env.GitRepository.CommitFile("infra/prometheus/subcomponent/component.cue", "undeploy subcomponent")
	assert.NilError(t, err)

	result, err = reconciler.Reconcile(env.Ctx, gProject)
	assert.NilError(t, err)
	inventoryStorage, err = reconciler.InventoryManager.Load()
	assert.NilError(t, err)
	assert.Assert(t, len(inventoryStorage.Manifests) == 1)
	assert.Assert(t, !inventoryStorage.Has(subComponentDeploymentManifest))
	assert.Assert(t, inventoryStorage.Has(mynamespace))
}

func TestReconciler_Reconcile_Suspend(t *testing.T) {
	env := projecttest.StartProjectEnv(t)
	defer env.Stop()
	cueCtx := cuecontext.New()
	chartReconciler := helm.ChartReconciler{
		Cfg: *env.HelmConfig,
		Log: env.Log,
	}

	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		CueContext:        cueCtx,
		RepositoryManager: env.RepositoryManager,
		ProjectManager:    env.ProjectManager,
		ChartReconciler:   chartReconciler,
		InventoryManager: inventory.Manager{
			Log:  env.Log,
			Path: filepath.Join(os.TempDir(), "inventory"),
		},
		Log: env.Log,
	}

	suspend := true
	result, err := reconciler.Reconcile(env.Ctx, gitopsv1.GitOpsProject{
		TypeMeta: v1.TypeMeta{
			APIVersion: "gitops.declcd.io/v1",
			Kind:       "GitOpsProject",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "reconcile-test",
			Namespace: "default",
			UID:       "reconcile-test",
		},
		Spec: gitopsv1.GitOpsProjectSpec{
			URL:                 env.TestProject,
			PullIntervalSeconds: 5,
			Suspend:             &suspend,
		},
	})
	assert.NilError(t, err)
	assert.Equal(t, result.Suspended, true)
	ctx := context.Background()
	var deployment appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: "mynamespace"}, &deployment)
	assert.Error(t, err, "deployments.apps \"mysubcomponent\" not found")
}
