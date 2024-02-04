package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestReconciler_Reconcile(t *testing.T) {
	env := projecttest.StartProjectEnv(t,
		projecttest.WithKubernetes(
			kubetest.WithHelm(true, false),
			kubetest.WithDecryptionKeyCreated(),
			kubetest.WithVCSSSHKeyCreated(),
		),
	)
	defer env.Stop()
	chartReconciler := helm.NewChartReconciler(
		env.HelmEnv.HelmConfig,
		env.DynamicTestKubeClient,
		"",
		env.InventoryManager,
		env.Log,
	)
	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		ComponentBuilder:  component.NewBuilder(),
		RepositoryManager: env.RepositoryManager,
		ProjectManager:    env.ProjectManager,
		ChartReconciler:   chartReconciler,
		InventoryManager:  env.InventoryManager,
		GarbageCollector:  env.GarbageCollector,
		Log:               env.Log,
		Decrypter:         env.SecretManager.Decrypter,
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
	ns := "prometheus"
	var mysubcomponent appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: ns}, &mysubcomponent)
	assert.NilError(t, err)
	assert.Equal(t, mysubcomponent.Name, "mysubcomponent")
	assert.Equal(t, mysubcomponent.Namespace, ns)
	var dep appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: ns}, &dep)
	assert.NilError(t, err)
	assert.Equal(t, dep.Name, "test")
	assert.Equal(t, dep.Namespace, ns)
	var sec corev1.Secret
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "secret", Namespace: ns}, &sec)
	assert.NilError(t, err)
	fooSecretValue, found := sec.Data["foo"]
	assert.Assert(t, found)
	assert.Equal(t, string(fooSecretValue), "bar")
	inventoryStorage, err := reconciler.InventoryManager.Load()
	assert.NilError(t, err)
	invComponents := inventoryStorage.Components()
	assert.Assert(t, len(invComponents) == 3)
	prometheus := invComponents["prometheus"]
	assert.Assert(t, len(prometheus.Items()) == 3)
	testHR := inventory.NewHelmReleaseItem(
		"prometheus",
		dep.Name,
		dep.Namespace,
	)
	assert.Assert(t, inventoryStorage.HasItem(testHR))
	invNs := inventory.NewManifest(
		v1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		"prometheus",
		mysubcomponent.Namespace,
		"",
	)
	assert.Assert(t, inventoryStorage.HasItem(invNs))
	subComponentDeploymentManifest := inventory.NewManifest(
		v1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		"subcomponent",
		mysubcomponent.Name,
		mysubcomponent.Namespace,
	)
	assert.Assert(t, inventoryStorage.HasItem(subComponentDeploymentManifest))
	t.Run("RemoveSubcomponent", func(t *testing.T) {
		err = os.Remove(filepath.Join(env.TestProject, "infra", "prometheus", "subcomponent", "component.cue"))
		assert.NilError(t, err)
		err = env.GitRepository.CommitFile("infra/prometheus/subcomponent/component.cue", "undeploy subcomponent")
		assert.NilError(t, err)
		result, err = reconciler.Reconcile(env.Ctx, gProject)
		assert.NilError(t, err)
		inventoryStorage, err := reconciler.InventoryManager.Load()
		assert.NilError(t, err)
		invComponents := inventoryStorage.Components()
		assert.Assert(t, len(invComponents) == 2)
		assert.Assert(t, !inventoryStorage.HasItem(subComponentDeploymentManifest))
		assert.Assert(t, inventoryStorage.HasItem(invNs))
		assert.Assert(t, inventoryStorage.HasItem(testHR))
		err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: ns}, &mysubcomponent)
		assert.Error(t, err, "deployments.apps \"mysubcomponent\" not found")
	})
	t.Run("RemovePrometheus", func(t *testing.T) {
		err = os.Remove(filepath.Join(env.TestProject, "infra", "prometheus", "component.cue"))
		assert.NilError(t, err)
		err = env.GitRepository.CommitFile("infra/prometheus/component.cue", "undeploy prometheus")
		assert.NilError(t, err)
		result, err = reconciler.Reconcile(env.Ctx, gProject)
		assert.NilError(t, err)
		inventoryStorage, err = reconciler.InventoryManager.Load()
		assert.NilError(t, err)
		invComponents := inventoryStorage.Components()
		assert.Assert(t, len(invComponents) == 1)
		assert.Assert(t, !inventoryStorage.HasItem(invNs))
		assert.Assert(t, !inventoryStorage.HasItem(testHR))
		err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: ns}, &dep)
		assert.Error(t, err, "deployments.apps \"test\" not found")
	})
}

func TestReconciler_Reconcile_Suspend(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false), kubetest.WithDecryptionKeyCreated()))
	defer env.Stop()
	chartReconciler := helm.NewChartReconciler(
		env.HelmEnv.HelmConfig,
		env.DynamicTestKubeClient,
		"",
		env.InventoryManager,
		env.Log,
	)
	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		ComponentBuilder:  component.NewBuilder(),
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
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"}, &deployment)
	assert.Error(t, err, "deployments.apps \"mysubcomponent\" not found")
}
