package garbage_test

import (
	"context"
	"testing"

	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCollector_Collect(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
	defer env.Stop()
	nsA := component.NewManifestMetadata(
		metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		"test",
		"a",
		"",
	)
	depA := component.NewManifestMetadata(
		metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		"test",
		"a",
		"a",
	)
	nsB := component.NewManifestMetadata(
		metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		"test",
		"b",
		"",
	)
	depB := component.NewManifestMetadata(
		metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		"test",
		"b",
		"b",
	)
	invManifests := []component.ManifestMetadata{
		nsA,
		depA,
		nsB,
		depB,
	}
	ctx := context.Background()
	converter := runtime.DefaultUnstructuredConverter
	client, err := kube.NewDynamicClient(env.ControlPlane.Config)
	assert.NilError(t, err)
	for _, im := range invManifests {
		obj, err := converter.ToUnstructured(toObject(im))
		unstr := unstructured.Unstructured{Object: obj}
		err = client.Apply(ctx, &unstr, "test")
		assert.NilError(t, err)
		err = env.InventoryManager.StoreManifest(im)
		assert.NilError(t, err)
	}

	hr := component.NewHelmReleaseMetadata(
		"test", "test", "test",
	)
	invHelmReleases := []component.HelmReleaseMetadata{
		hr,
	}
	helmReconciler := helm.ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: env.Log,
	}
	releases := make([]helm.Release, 0, len(invHelmReleases))
	for _, hr := range invHelmReleases {
		chart := helm.Chart{
			Name:    hr.Name(),
			RepoURL: env.HelmEnv.RepositoryServer.URL,
			Version: "1.0.0",
		}

		_, err := helmReconciler.Reconcile(chart, helm.Namespace(hr.Namespace()))
		assert.NilError(t, err)
		releases = append(releases, helm.Release{
			Name:      hr.Name(),
			Namespace: hr.Namespace(),
		})
		err = env.InventoryManager.StoreHelmRelease(hr)
		assert.NilError(t, err)
	}
	storage, err := env.InventoryManager.Load()
	assert.NilError(t, err)
	assert.Assert(t, storage.HasManifest(nsA))
	assert.Assert(t, storage.HasManifest(depA))
	assert.Assert(t, storage.HasManifest(nsB))
	assert.Assert(t, storage.HasManifest(depB))
	assert.Assert(t, storage.HasRelease(hr))
	collector := env.GarbageCollector
	dag := component.NewDependencyGraph()
	dag.Insert(component.NewNode("test", "", []string{}, invManifests, invHelmReleases))
	err = collector.Collect(ctx, dag)
	assert.NilError(t, err)
	var deploymentA appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "a", Namespace: "a"}, &deploymentA)
	assert.NilError(t, err)
	assert.Equal(t, deploymentA.Name, "a")
	assert.Equal(t, deploymentA.Namespace, "a")
	var deploymentB appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "b", Namespace: "b"}, &deploymentB)
	assert.NilError(t, err)
	assert.Equal(t, deploymentB.Name, "b")
	assert.Equal(t, deploymentB.Namespace, "b")
	var hrDeployment appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: "test"}, &hrDeployment)
	assert.NilError(t, err)
	assert.Equal(t, hrDeployment.Name, "test")
	assert.Equal(t, hrDeployment.Namespace, "test")

	t.Run("Changes", func(t *testing.T) {
		renderedManifests := []component.ManifestMetadata{
			nsA,
			nsB,
			depA,
		}
		dag := component.NewDependencyGraph()
		dag.Insert(component.NewNode("test", "", []string{}, renderedManifests, []component.HelmReleaseMetadata{}))
		err = collector.Collect(ctx, dag)
		assert.NilError(t, err)
		var deploymentA appsv1.Deployment
		err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "a", Namespace: "a"}, &deploymentA)
		assert.NilError(t, err)
		assert.Equal(t, deploymentA.Name, "a")
		assert.Equal(t, deploymentA.Namespace, "a")
		var deploymentB appsv1.Deployment
		err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "b", Namespace: "b"}, &deploymentB)
		assert.Error(t, err, "deployments.apps \"b\" not found")
		var hrDeployment appsv1.Deployment
		err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: "test"}, &hrDeployment)
		assert.Error(t, err, "deployments.apps \"test\" not found")
	})
}

func toObject(invManifest component.ManifestMetadata) client.Object {
	switch invManifest.Kind {
	case "Deployment":
		return deployment(invManifest)
	case "Namespace":
		return namespace(invManifest)
	}

	return nil
}

func namespace(invManifest component.ManifestMetadata) client.Object {
	return &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: invManifest.Name(),
		},
	}
}

func deployment(invManifest component.ManifestMetadata) client.Object {
	replicas := int32(1)
	labels := map[string]string{
		"app": invManifest.Name(),
	}
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      invManifest.Name(),
			Namespace: invManifest.Namespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  invManifest.Name(),
							Image: "test",
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("10m"),
									v1.ResourceMemory: resource.MustParse("10Mi"),
								},
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("10m"),
									v1.ResourceMemory: resource.MustParse("10Mi"),
								},
							},
						},
					},
				}},
		},
	}

}
