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

package garbage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/kharf/declcd/internal/helmtest"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	helmEnvironment *helmtest.Environment
)

func TestMain(m *testing.M) {
	var err error
	helmEnvironment, err = helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assertError(err)
	defer helmEnvironment.Close()
}

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func TestCollector_Collect(t *testing.T) {
	nsA := &inventory.ManifestItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		Name: "a",
		ID:   "a___Namespace",
	}
	depA := &inventory.ManifestItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Name:      "a",
		Namespace: "a",
		ID:        "a_a_apps_Deployment",
	}
	nsB := &inventory.ManifestItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		Name: "b",
		ID:   "b___Namespace",
	}
	depB := &inventory.ManifestItem{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Name:      "b",
		Namespace: "b",
		ID:        "b_b_apps_Deployment",
	}
	invManifests := []*inventory.ManifestItem{
		nsA,
		depA,
		nsB,
		depB,
	}

	hr := &inventory.HelmReleaseItem{
		Name:      "test",
		Namespace: "test",
		ID:        "test_test_HelmRelease",
	}
	invHelmReleases := []*inventory.HelmReleaseItem{
		hr,
	}

	testCases := []struct {
		name    string
		runCase func(context.Context, projecttest.Environment)
	}{
		{
			name: "Deleted-DepB-and-HR",
			runCase: func(ctx context.Context, env projecttest.Environment) {
				dag := component.NewDependencyGraph()
				prepareManifests(ctx, t, invManifests, env, dag)

				chartReconciler := helm.ChartReconciler{
					KubeConfig:            env.ControlPlane.Config,
					Client:                env.DynamicTestKubeClient,
					FieldManager:          "controller",
					InventoryManager:      env.InventoryManager,
					InsecureSkipTLSverify: true,
					Log:                   env.Log,
				}
				prepareHelmReleases(ctx, t, invHelmReleases, env, chartReconciler, dag)

				storage, err := env.InventoryManager.Load()
				assert.NilError(t, err)

				assertItems(t, invManifests, invHelmReleases, storage)
				assertRunningAll := func(ctx context.Context, t *testing.T, env projecttest.Environment) {
					assertRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "a",
								"namespace": "a",
							},
						},
					})

					assertRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "b",
								"namespace": "b",
							},
						},
					})

					assertRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "test",
								"namespace": "test",
							},
						},
					})
				}
				assertRunningAll(ctx, t, env)

				err = env.GarbageCollector.Collect(ctx, &dag)
				assert.NilError(t, err)

				storage, err = env.InventoryManager.Load()
				assert.NilError(t, err)

				assertItems(t, invManifests, invHelmReleases, storage)
				assertRunningAll(ctx, t, env)

				renderedManifests := []*inventory.ManifestItem{
					nsA,
					nsB,
					depA,
				}

				dag = component.NewDependencyGraph()
				prepareManifests(ctx, t, renderedManifests, env, dag)

				err = env.GarbageCollector.Collect(ctx, &dag)
				assert.NilError(t, err)

				assertRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "a",
							"namespace": "a",
						},
					},
				})

				assertNotRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "b",
							"namespace": "b",
						},
					},
				})

				assertNotRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "test",
						},
					},
				})
			},
		},
		{
			name: "Deleted-Deployment-But-Still-In-Inventory",
			runCase: func(ctx context.Context, env projecttest.Environment) {
				renderedManifests := []*inventory.ManifestItem{
					nsA,
					depA,
				}

				dag := component.NewDependencyGraph()
				prepareManifests(ctx, t, renderedManifests, env, dag)
				storage, err := env.InventoryManager.Load()
				assert.NilError(t, err)
				assertItems(t, renderedManifests, []*inventory.HelmReleaseItem{}, storage)

				obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(toObject(depA))
				assert.NilError(t, err)
				unstr := &unstructured.Unstructured{Object: obj}
				assertRunning(ctx, t, env.DynamicTestKubeClient, unstr)

				err = env.DynamicTestKubeClient.Delete(ctx, unstr)
				assert.NilError(t, err)

				storage, err = env.InventoryManager.Load()
				assert.NilError(t, err)
				assertItems(t, renderedManifests, []*inventory.HelmReleaseItem{}, storage)

				err = env.GarbageCollector.Collect(ctx, &dag)
				assert.NilError(t, err)

				storage, err = env.InventoryManager.Load()
				assert.NilError(t, err)

				assertNotRunning(ctx, t, env.DynamicTestKubeClient, &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "a",
							"namespace": "a",
						},
					},
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := projecttest.StartProjectEnv(
				t,
			)
			defer env.Stop()

			ctx := context.Background()
			tc.runCase(ctx, env)
		})
	}
}

func assertRunning(
	ctx context.Context,
	t *testing.T,
	client *kube.DynamicClient,
	obj *unstructured.Unstructured,
) {
	foundObj, err := client.Get(ctx, obj)
	assert.NilError(t, err)
	assert.Equal(t, foundObj.GetName(), obj.GetName())
	assert.Equal(t, foundObj.GetNamespace(), obj.GetNamespace())
}

func assertNotRunning(
	ctx context.Context,
	t *testing.T,
	client *kube.DynamicClient,
	obj *unstructured.Unstructured,
) {
	restMapper := client.RESTMapper()
	gvk := obj.GroupVersionKind()
	mapping, err := restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	assert.NilError(t, err)
	_, err = client.Get(ctx, obj)
	assert.Error(
		t,
		err,
		fmt.Sprintf(
			"%s.%s \"%s\" not found",
			mapping.Resource.Resource,
			mapping.Resource.Group,
			obj.GetName(),
		),
	)
}

func assertItems(
	t *testing.T,
	manifests []*inventory.ManifestItem,
	releases []*inventory.HelmReleaseItem,
	storage *inventory.Storage,
) {
	for _, manifest := range manifests {
		assert.Assert(t, storage.HasItem(manifest))
	}
	for _, release := range releases {
		assert.Assert(t, storage.HasItem(release))
	}
}

func prepareHelmReleases(
	ctx context.Context,
	t *testing.T,
	invHelmReleases []*inventory.HelmReleaseItem,
	env projecttest.Environment,
	chartReconciler helm.ChartReconciler,
	dag component.DependencyGraph,
) {
	releases := make([]helm.ReleaseDeclaration, 0, len(invHelmReleases))
	for _, hrMetadata := range invHelmReleases {
		release := helm.ReleaseDeclaration{
			Name:      hrMetadata.GetName(),
			Namespace: hrMetadata.GetNamespace(),
			Chart: helm.Chart{
				Name:    "test",
				RepoURL: helmEnvironment.ChartServer.URL(),
				Version: "1.0.0",
			},
			Values: helm.Values{},
		}
		id := fmt.Sprintf("%s_%s_HelmRelease", release.Name, release.Namespace)
		_, err := chartReconciler.Reconcile(ctx, release, id)
		assert.NilError(t, err)
		releases = append(releases, release)
		err = env.InventoryManager.StoreItem(&inventory.HelmReleaseItem{
			Name:      release.Name,
			Namespace: release.Namespace,
			ID:        id,
		}, nil)
		assert.NilError(t, err)
		dag.Insert(&component.HelmRelease{
			ID:           hrMetadata.ID,
			Dependencies: []string{},
			Content:      release,
		})
	}
}

func prepareManifests(
	ctx context.Context,
	t *testing.T,
	invManifests []*inventory.ManifestItem,
	env projecttest.Environment,
	dag component.DependencyGraph,
) {
	for _, im := range invManifests {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(toObject(im))
		unstr := unstructured.Unstructured{Object: obj}
		err = env.DynamicTestKubeClient.Apply(ctx, &unstr, "test")
		assert.NilError(t, err)
		buf := &bytes.Buffer{}
		json.NewEncoder(buf).Encode(unstr.Object)
		err = env.InventoryManager.StoreItem(im, buf)
		assert.NilError(t, err)
		dag.Insert(
			&component.Manifest{
				ID:           im.ID,
				Dependencies: []string{},
				Content:      unstr,
			},
		)
	}
}

func toObject(invManifest *inventory.ManifestItem) client.Object {
	switch invManifest.TypeMeta.Kind {
	case "Deployment":
		return deployment(invManifest)
	case "Namespace":
		return namespace(invManifest)
	}
	return nil
}

func namespace(invManifest *inventory.ManifestItem) client.Object {
	return &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: invManifest.GetName(),
		},
	}
}

func deployment(invManifest *inventory.ManifestItem) client.Object {
	replicas := int32(1)
	labels := map[string]string{
		"app": invManifest.GetName(),
	}
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      invManifest.GetName(),
			Namespace: invManifest.GetNamespace(),
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
							Name:  invManifest.GetName(),
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

var errResult error

func BenchmarkCollector_Collect(b *testing.B) {
	env := projecttest.StartProjectEnv(
		b,
	)
	defer env.Stop()
	dag := component.NewDependencyGraph()
	converter := runtime.DefaultUnstructuredConverter
	for i := 0; i < 1000; i++ {
		name := "component-" + strconv.Itoa(i)
		nsItem := &inventory.ManifestItem{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			Name: name,
			ID:   "",
		}
		err := env.InventoryManager.StoreItem(nsItem, nil)
		assert.NilError(b, err)
		obj, err := converter.ToUnstructured(toObject(nsItem))
		unstr := unstructured.Unstructured{Object: obj}
		err = env.DynamicTestKubeClient.Apply(env.Ctx, &unstr, "test")
		assert.NilError(b, err)
		depItem := &inventory.ManifestItem{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			Name:      name,
			Namespace: name,
			ID:        fmt.Sprintf("%s_%s_apps_Deployment", name, name),
		}
		err = env.InventoryManager.StoreItem(depItem, nil)
		assert.NilError(b, err)
		obj, err = converter.ToUnstructured(toObject(depItem))
		unstr = unstructured.Unstructured{Object: obj}
		err = env.DynamicTestKubeClient.Apply(env.Ctx, &unstr, "test")
		assert.NilError(b, err)
		nsNode := &component.Manifest{
			Dependencies: []string{},
			Content: unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       nsItem.TypeMeta.Kind,
					"apiVersion": nsItem.TypeMeta.APIVersion,
					"metadata": map[string]interface{}{
						"name": nsItem.GetName(),
					},
				},
			},
		}
		err = dag.Insert(nsNode)
		assert.NilError(b, err)
	}
	var err error
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		err = env.GarbageCollector.Collect(env.Ctx, &dag)
	}
	errResult = err
}
