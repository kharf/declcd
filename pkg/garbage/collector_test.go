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
	"path/filepath"
	goRuntime "runtime"
	"testing"

	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/garbage"
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

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type testCaseContext struct {
	ctx               context.Context
	env               projecttest.Environment
	inventoryInstance *inventory.Instance
	collector         garbage.Collector
}

func TestCollector_Collect(t *testing.T) {
	var err error
	helmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assertError(err)
	defer helmEnvironment.Close()

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
		runCase func(context testCaseContext)
	}{
		{
			name: "Deleted-DepB-and-HR",
			runCase: func(context testCaseContext) {
				dag := component.NewDependencyGraph()
				ctx := context.ctx
				env := context.env
				inventoryInstance := context.inventoryInstance

				prepareManifests(ctx,
					t,
					invManifests,
					env,
					inventoryInstance,
					dag,
				)

				chartReconciler := helm.ChartReconciler{
					KubeConfig:            env.ControlPlane.Config,
					Client:                env.DynamicTestKubeClient,
					FieldManager:          "controller",
					InsecureSkipTLSverify: true,
					InventoryInstance:     inventoryInstance,
					Log:                   env.Log,
				}

				prepareHelmReleases(
					ctx,
					t,
					helmEnvironment,
					invHelmReleases,
					chartReconciler,
					inventoryInstance,
					dag,
				)

				storage, err := inventoryInstance.Load()
				assert.NilError(t, err)

				assertItems(t, invManifests, invHelmReleases, storage)
				assertRunningAll := func(t *testing.T) {
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
				assertRunningAll(t)

				err = context.collector.Collect(ctx, &dag)
				assert.NilError(t, err)

				storage, err = inventoryInstance.Load()
				assert.NilError(t, err)

				assertItems(t, invManifests, invHelmReleases, storage)
				assertRunningAll(t)

				renderedManifests := []*inventory.ManifestItem{
					nsA,
					nsB,
					depA,
				}

				dag = component.NewDependencyGraph()
				prepareManifests(ctx, t, renderedManifests, env, inventoryInstance, dag)

				err = context.collector.Collect(ctx, &dag)
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
			runCase: func(context testCaseContext) {
				renderedManifests := []*inventory.ManifestItem{
					nsA,
					depA,
				}

				dag := component.NewDependencyGraph()
				ctx := context.ctx
				env := context.env
				inventoryInstance := context.inventoryInstance

				prepareManifests(ctx, t, renderedManifests, env, inventoryInstance, dag)
				storage, err := inventoryInstance.Load()
				assert.NilError(t, err)
				assertItems(t, renderedManifests, []*inventory.HelmReleaseItem{}, storage)

				obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(toObject(depA))
				assert.NilError(t, err)
				unstr := &unstructured.Unstructured{Object: obj}
				assertRunning(ctx, t, env.DynamicTestKubeClient, unstr)

				err = env.DynamicTestKubeClient.Delete(ctx, unstr)
				assert.NilError(t, err)

				storage, err = inventoryInstance.Load()
				assert.NilError(t, err)
				assertItems(t, renderedManifests, []*inventory.HelmReleaseItem{}, storage)

				err = context.collector.Collect(ctx, &dag)
				assert.NilError(t, err)

				storage, err = inventoryInstance.Load()
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

			inventoryInstance := &inventory.Instance{
				Path: filepath.Join(env.TestRoot, "inventory"),
			}

			collector := garbage.Collector{
				Log:               env.Log,
				Client:            env.DynamicTestKubeClient,
				KubeConfig:        env.ControlPlane.Config,
				InventoryInstance: inventoryInstance,
				WorkerPoolSize:    goRuntime.GOMAXPROCS(0),
			}

			ctx := context.Background()
			tc.runCase(testCaseContext{
				ctx:               ctx,
				env:               env,
				inventoryInstance: inventoryInstance,
				collector:         collector,
			})
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
	helmEnvironment *helmtest.Environment,
	invHelmReleases []*inventory.HelmReleaseItem,
	chartReconciler helm.ChartReconciler,
	inventoryInstance *inventory.Instance,
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
		_, err := chartReconciler.Reconcile(
			ctx,
			&helm.ReleaseComponent{
				ID:      id,
				Content: release,
			},
		)
		assert.NilError(t, err)
		releases = append(releases, release)
		err = inventoryInstance.StoreItem(&inventory.HelmReleaseItem{
			Name:      release.Name,
			Namespace: release.Namespace,
			ID:        id,
		}, nil)
		assert.NilError(t, err)
		dag.Insert(&helm.ReleaseComponent{
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
	inventoryInstance *inventory.Instance,
	dag component.DependencyGraph,
) {
	for _, im := range invManifests {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(toObject(im))
		unstr := unstructured.Unstructured{Object: obj}
		err = env.DynamicTestKubeClient.Apply(ctx, &unstr, "test")
		assert.NilError(t, err)
		buf := &bytes.Buffer{}
		json.NewEncoder(buf).Encode(unstr.Object)
		err = inventoryInstance.StoreItem(im, buf)
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
