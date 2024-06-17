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

package project_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/cloudtest"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/cloud"
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

var (
	publicHelmEnvironment *helmtest.Environment
	azureHelmEnvironment  *helmtest.Environment
)

func TestMain(m *testing.M) {
	var err error
	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	registryPath, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assertError(err)
	defer cueModuleRegistry.Close()

	publicHelmEnvironment, err = helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assertError(err)
	defer publicHelmEnvironment.Close()

	azureHelmEnvironment, err = helmtest.NewHelmEnvironment(
		helmtest.WithOCI(true),
		helmtest.WithPrivate(true),
		helmtest.WithProvider(cloud.Azure),
	)
	assertError(err)
	defer azureHelmEnvironment.Close()
	azureCloudEnvironment, err := cloudtest.NewAzureEnvironment()
	assertError(err)
	defer azureCloudEnvironment.Close()
}

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func TestReconciler_Reconcile(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, env projecttest.Environment, reconciler project.Reconciler, gProject gitops.GitOpsProject)
	}{
		{
			name: "Simple",
			run: func(t *testing.T, env projecttest.Environment, reconciler project.Reconciler, gProject gitops.GitOpsProject) {
				result, err := reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				assert.Equal(t, result.Suspended, false)

				ctx := context.Background()
				ns := "prometheus"
				var mysubcomponent appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: ns},
					&mysubcomponent,
				)

				assert.NilError(t, err)
				assert.Equal(t, mysubcomponent.Name, "mysubcomponent")
				assert.Equal(t, mysubcomponent.Namespace, ns)

				var dep appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "test", Namespace: ns},
					&dep,
				)
				assert.NilError(t, err)
				assert.Equal(t, dep.Name, "test")
				assert.Equal(t, dep.Namespace, ns)

				var sec corev1.Secret
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "secret", Namespace: ns},
					&sec,
				)
				assert.NilError(t, err)
				fooSecretValue, found := sec.Data["foo"]
				assert.Assert(t, found)
				assert.Equal(t, string(fooSecretValue), "bar")

				inventoryStorage, err := reconciler.InventoryManager.Load()
				assert.NilError(t, err)

				invComponents := inventoryStorage.Items()
				assert.Assert(t, len(invComponents) == 5)
				testHR := &inventory.HelmReleaseItem{
					Name:      dep.Name,
					Namespace: dep.Namespace,
					ID:        fmt.Sprintf("%s_%s_HelmRelease", dep.Name, dep.Namespace),
				}
				assert.Assert(t, inventoryStorage.HasItem(testHR))

				invNs := &inventory.ManifestItem{
					TypeMeta: v1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					Name:      mysubcomponent.Namespace,
					Namespace: "",
					ID:        fmt.Sprintf("%s___Namespace", mysubcomponent.Namespace),
				}
				assert.Assert(t, inventoryStorage.HasItem(invNs))

				subComponentDeploymentManifest := &inventory.ManifestItem{
					TypeMeta: v1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					Name:      mysubcomponent.Name,
					Namespace: mysubcomponent.Namespace,
					ID: fmt.Sprintf(
						"%s_%s_apps_Deployment",
						mysubcomponent.Name,
						mysubcomponent.Namespace,
					),
				}
				assert.Assert(t, inventoryStorage.HasItem(subComponentDeploymentManifest))

				err = os.RemoveAll(
					filepath.Join(env.TestProject, "infra", "prometheus", "subcomponent"),
				)
				assert.NilError(t, err)
				_, err = env.GitRepository.CommitFile(
					"infra/prometheus/",
					"undeploy subcomponent",
				)
				assert.NilError(t, err)
				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				inventoryStorage, err = reconciler.InventoryManager.Load()
				assert.NilError(t, err)
				invComponents = inventoryStorage.Items()
				assert.Assert(t, len(invComponents) == 4)
				assert.Assert(t, !inventoryStorage.HasItem(subComponentDeploymentManifest))
				assert.Assert(t, inventoryStorage.HasItem(invNs))
				assert.Assert(t, inventoryStorage.HasItem(testHR))
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: ns},
					&mysubcomponent,
				)
				assert.Error(t, err, "deployments.apps \"mysubcomponent\" not found")
			},
		},
		{
			name: "WorkloadIdentity",
			run: func(t *testing.T, env projecttest.Environment, reconciler project.Reconciler, gProject gitops.GitOpsProject) {
				result, err := reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				assert.Equal(t, result.Suspended, false)

				ctx := context.Background()
				ns := "prometheus"

				var dep appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "test", Namespace: ns},
					&dep,
				)
				assert.NilError(t, err)
				assert.Equal(t, dep.Name, "test")
				assert.Equal(t, dep.Namespace, ns)

				inventoryStorage, err := reconciler.InventoryManager.Load()
				assert.NilError(t, err)

				invComponents := inventoryStorage.Items()
				assert.Assert(t, len(invComponents) == 2)
				testHR := &inventory.HelmReleaseItem{
					Name:      dep.Name,
					Namespace: dep.Namespace,
					ID:        fmt.Sprintf("%s_%s_HelmRelease", dep.Name, dep.Namespace),
				}
				assert.Assert(t, inventoryStorage.HasItem(testHR))
			},
		},
		{
			name: "Suspend",
			run: func(t *testing.T, env projecttest.Environment, reconciler project.Reconciler, gProject gitops.GitOpsProject) {
				suspend := true
				gProject.Spec.Suspend = &suspend
				result, err := reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				assert.Equal(t, result.Suspended, true)

				ctx := context.Background()
				var deployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"},
					&deployment,
				)
				assert.Error(t, err, "deployments.apps \"mysubcomponent\" not found")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := projecttest.StartProjectEnv(t,
				projecttest.WithKubernetes(
					kubetest.WithDecryptionKeyCreated(),
					kubetest.WithVCSSSHKeyCreated(),
				),
			)
			defer env.Stop()

			chartReconciler := helm.ChartReconciler{
				KubeConfig:            env.ControlPlane.Config,
				Client:                env.DynamicTestKubeClient,
				FieldManager:          "controller",
				InventoryManager:      env.InventoryManager,
				InsecureSkipTLSverify: true,
				Log:                   env.Log,
			}

			reconciler := project.Reconciler{
				Client:            env.ControllerManager.GetClient(),
				DynamicClient:     env.DynamicTestKubeClient,
				ComponentBuilder:  component.NewBuilder(),
				RepositoryManager: env.RepositoryManager,
				ProjectManager:    env.ProjectManager,
				ChartReconciler:   chartReconciler,
				InventoryManager:  env.InventoryManager,
				GarbageCollector:  env.GarbageCollector,
				Log:               env.Log,
				FieldManager:      project.ControllerName,
				WorkerPoolSize:    runtime.GOMAXPROCS(0),
			}

			suspend := false
			gProject := gitops.GitOpsProject{
				TypeMeta: v1.TypeMeta{
					APIVersion: "gitops.declcd.io/v1",
					Kind:       "GitOpsProject",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      env.TestRoot,
					Namespace: "default",
					UID:       types.UID(env.TestRoot),
				},
				Spec: gitops.GitOpsProjectSpec{
					URL:                 env.TestProject,
					PullIntervalSeconds: 5,
					Suspend:             &suspend,
				},
			}

			tc.run(t, env, reconciler, gProject)
		})
	}
}

var reconcileResult *project.ReconcileResult

func BenchmarkReconciler_Reconcile(b *testing.B) {
	env := projecttest.StartProjectEnv(b,
		projecttest.WithKubernetes(
			kubetest.WithDecryptionKeyCreated(),
			kubetest.WithVCSSSHKeyCreated(),
		),
	)
	defer env.Stop()
	chartReconciler := helm.ChartReconciler{
		KubeConfig:            env.ControlPlane.Config,
		Client:                env.DynamicTestKubeClient,
		FieldManager:          "controller",
		InventoryManager:      env.InventoryManager,
		InsecureSkipTLSverify: true,
		Log:                   env.Log,
	}
	reconciler := project.Reconciler{
		Client:            env.ControllerManager.GetClient(),
		DynamicClient:     env.DynamicTestKubeClient,
		ComponentBuilder:  component.NewBuilder(),
		RepositoryManager: env.RepositoryManager,
		ProjectManager:    env.ProjectManager,
		ChartReconciler:   chartReconciler,
		InventoryManager:  env.InventoryManager,
		GarbageCollector:  env.GarbageCollector,
		Log:               env.Log,
		FieldManager:      project.ControllerName,
		WorkerPoolSize:    runtime.GOMAXPROCS(0),
	}
	suspend := false
	gProject := gitops.GitOpsProject{
		TypeMeta: v1.TypeMeta{
			APIVersion: "gitops.declcd.io/v1",
			Kind:       "GitOpsProject",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "reconcile-test",
			Namespace: "default",
			UID:       "reconcile-test",
		},
		Spec: gitops.GitOpsProjectSpec{
			URL:                 env.TestProject,
			PullIntervalSeconds: 5,
			Suspend:             &suspend,
		},
	}
	var err error
	var result *project.ReconcileResult
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err = reconciler.Reconcile(env.Ctx, gProject)
		assert.NilError(b, err)
		assert.Equal(b, result.Suspended, false)
	}
	reconcileResult = result
}
