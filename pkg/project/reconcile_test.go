package project_test

import (
	"context"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/helm"
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
		Log:               env.Log,
	}

	suspend := false
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
		Log:               env.Log,
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
