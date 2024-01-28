package garbage_test

import (
	"context"
	"testing"

	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
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
	nsA := inventory.NewManifest(
		metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		"test",
		"a",
		"",
	)
	depA := inventory.NewManifest(
		metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		"test",
		"a",
		"a",
	)
	nsB := inventory.NewManifest(
		metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		"test",
		"b",
		"",
	)
	depB := inventory.NewManifest(
		metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		"test",
		"b",
		"b",
	)
	invManifests := []inventory.Manifest{
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
		err = env.InventoryManager.StoreItem(im, nil)
		assert.NilError(t, err)
	}
	hr := inventory.NewHelmReleaseItem(
		"test", "test", "test",
	)
	invHelmReleases := []inventory.HelmReleaseItem{
		hr,
	}
	chartReconciler := helm.NewChartReconciler(
		env.HelmEnv.HelmConfig,
		env.DynamicTestKubeClient,
		"",
		env.InventoryManager,
		env.Log,
	)
	releases := make([]helm.ReleaseDeclaration, 0, len(invHelmReleases))
	for _, hrMetadata := range invHelmReleases {
		release := helm.ReleaseDeclaration{
			Name:      hrMetadata.Name(),
			Namespace: hrMetadata.Namespace(),
			Chart: helm.Chart{
				Name:    "test",
				RepoURL: env.HelmEnv.RepositoryServer.URL,
				Version: "1.0.0",
			},
			Values: helm.Values{},
		}
		_, err := chartReconciler.Reconcile(ctx, hrMetadata.ComponentID(), release)
		assert.NilError(t, err)
		releases = append(releases, release)
		err = env.InventoryManager.StoreItem(inventory.NewHelmReleaseItem(
			hrMetadata.ComponentID(),
			release.Name,
			release.Namespace,
		), nil)
		assert.NilError(t, err)
	}
	storage, err := env.InventoryManager.Load()
	assert.NilError(t, err)
	assert.Assert(t, storage.HasItem(nsA))
	assert.Assert(t, storage.HasItem(depA))
	assert.Assert(t, storage.HasItem(nsB))
	assert.Assert(t, storage.HasItem(depB))
	assert.Assert(t, storage.HasItem(hr))
	collector := env.GarbageCollector
	dag := component.NewDependencyGraph()
	dag.Insert(component.NewNode("test", "", []string{}, toManifestMetadata(invManifests), toReleaseMetadata(invHelmReleases)))
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
		renderedManifests := []inventory.Manifest{
			nsA,
			nsB,
			depA,
		}
		dag := component.NewDependencyGraph()
		dag.Insert(component.NewNode("test", "", []string{}, toManifestMetadata(renderedManifests), []helm.ReleaseMetadata{}))
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

func toReleaseMetadata(items []inventory.HelmReleaseItem) []helm.ReleaseMetadata {
	metadata := make([]helm.ReleaseMetadata, 0, len(items))
	for _, item := range items {
		metadata = append(metadata, helm.NewReleaseMetadata(item.ComponentID(), item.Name(), item.Namespace()))
	}
	return metadata
}

func toManifestMetadata(items []inventory.Manifest) []kube.ManifestMetadata {
	metadata := make([]kube.ManifestMetadata, 0, len(items))
	for _, item := range items {
		metadata = append(metadata, kube.NewManifestMetadata(*item.TypeMeta(), item.ComponentID(), item.Name(), item.Namespace()))
	}
	return metadata
}

func toObject(invManifest inventory.Manifest) client.Object {
	switch invManifest.TypeMeta().Kind {
	case "Deployment":
		return deployment(invManifest)
	case "Namespace":
		return namespace(invManifest)
	}
	return nil
}

func namespace(invManifest inventory.Manifest) client.Object {
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

func deployment(invManifest inventory.Manifest) client.Object {
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
