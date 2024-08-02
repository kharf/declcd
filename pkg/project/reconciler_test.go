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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
	"unsafe"

	"cuelang.org/go/mod/modfile"
	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/cloudtest"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	"helm.sh/helm/v3/pkg/time"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type testCaseContext struct {
	environment       *projecttest.Environment
	reconciler        project.Reconciler
	gitopsProject     gitops.GitOpsProject
	inventoryInstance *inventory.Instance
}

func TestReconciler_Reconcile(t *testing.T) {
	var err error
	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	registryPath, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assertError(err)
	defer cueModuleRegistry.Close()

	publicHelmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assertError(err)
	defer publicHelmEnvironment.Close()

	azureHelmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(true),
		helmtest.WithPrivate(true),
		helmtest.WithProvider(cloud.Azure),
	)
	assertError(err)
	defer azureHelmEnvironment.Close()
	azureCloudEnvironment, err := cloudtest.NewAzureEnvironment()
	assertError(err)
	defer azureCloudEnvironment.Close()

	testCases := []struct {
		name    string
		prepare func() *projecttest.Environment
		run     func(t *testing.T, tcContext testCaseContext)
	}{
		{
			name: "Simple",
			prepare: func() *projecttest.Environment {
				return nil
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject

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

				inventoryStorage, err := tcContext.inventoryInstance.Load()
				assert.NilError(t, err)

				invComponents := inventoryStorage.Items()
				assert.Assert(t, len(invComponents) == 6)
				testHR := &inventory.HelmReleaseItem{
					Name:      dep.Name,
					Namespace: dep.Namespace,
					ID:        fmt.Sprintf("%s_%s_HelmRelease", dep.Name, dep.Namespace),
				}
				assert.Assert(t, inventoryStorage.HasItem(testHR))

				contentReader, err := tcContext.inventoryInstance.GetItem(testHR)
				defer contentReader.Close()

				storedBytes, err := io.ReadAll(contentReader)
				assert.NilError(t, err)

				desiredRelease := helm.Release{
					Name:      testHR.Name,
					Namespace: testHR.Namespace,
					CRDs: helm.CRDs{
						AllowUpgrade: true,
					},
					Chart: helm.Chart{
						Name:    "test",
						RepoURL: publicHelmEnvironment.ChartServer.URL(),
						Version: "1.0.0",
						Auth:    nil,
					},
					Values: helm.Values{
						"autoscaling": map[string]interface{}{
							"enabled": true,
						},
					},
					Patches: &helm.Patches{
						Unstructureds: map[string]kube.ExtendedUnstructured{
							"apps/v1-Deployment-prometheus-test": {
								Unstructured: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"apiVersion": "apps/v1",
										"kind":       "Deployment",
										"metadata": map[string]any{
											"name":      testHR.Name,
											"namespace": testHR.Namespace,
										},
										"spec": map[string]any{
											"replicas": int64(5),
											"template": map[string]any{
												"spec": map[string]any{
													"containers": []any{
														map[string]any{
															"name":  "prometheus",
															"image": "prometheus:1.14.2",
															"ports": []any{
																map[string]any{
																	"containerPort": int64(
																		80,
																	),
																},
															},
														},
														map[string]any{
															"name":  "sidecar",
															"image": "sidecar:1.14.2",
															"ports": []any{
																map[string]any{
																	"containerPort": int64(
																		80,
																	),
																},
															},
														},
													},
												},
											},
										},
									},
								},
								Metadata: &kube.ManifestMetadataNode{
									"spec": &kube.ManifestMetadataNode{
										"replicas": &kube.ManifestFieldMetadata{
											IgnoreAttr: kube.OnConflict,
										},
										"template": &kube.ManifestMetadataNode{
											"spec": &kube.ManifestMetadataNode{
												"containers": &kube.ManifestFieldMetadata{
													IgnoreAttr: kube.OnConflict,
												},
											},
										},
									},
								},
							},
						},
					},
				}

				desiredBuf := &bytes.Buffer{}
				err = json.NewEncoder(desiredBuf).Encode(desiredRelease)
				assert.NilError(t, err)

				assert.Equal(t, string(storedBytes), desiredBuf.String())

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

				testProject := env.Projects[0]
				err = os.RemoveAll(
					filepath.Join(testProject.TargetPath, "infra", "prometheus", "subcomponent"),
				)
				assert.NilError(t, err)
				_, err = testProject.GitRepository.CommitFile(
					"infra/prometheus/",
					"undeploy subcomponent",
				)
				assert.NilError(t, err)
				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				inventoryStorage, err = tcContext.inventoryInstance.Load()
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
			name: "Impersonation",
			prepare: func() *projecttest.Environment {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithProjectSource("mini"),
					projecttest.WithKubernetes(
						kubetest.WithVCSAuthSecretFor("mini"),
					),
				)
				err = helmtest.ReplaceTemplate(
					helmtest.Template{
						Name:                    "test",
						TestProjectPath:         env.Projects[0].TargetPath,
						RelativeReleaseFilePath: "infra/monitoring/releases.cue",
						RepoURL:                 publicHelmEnvironment.ChartServer.URL(),
					},
					env.Projects[0].GitRepository,
				)
				return &env
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject
				gProject.Namespace = "tenant"
				gProject.Spec.ServiceAccountName = "mysa"

				result, err := reconciler.Reconcile(env.Ctx, gProject)
				assert.Assert(
					t,
					strings.Contains(
						err.Error(),
						`is forbidden: User "system:serviceaccount:tenant:mysa" cannot patch resource`,
					),
				)

				namespace := corev1.Namespace{
					TypeMeta: v1.TypeMeta{
						APIVersion: "",
						Kind:       "Namespace",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "tenant",
					},
				}

				err = env.TestKubeClient.Create(env.Ctx, &namespace)
				assert.NilError(t, err)

				namespace = corev1.Namespace{
					TypeMeta: v1.TypeMeta{
						APIVersion: "",
						Kind:       "Namespace",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "monitoring",
					},
				}

				err = env.TestKubeClient.Create(env.Ctx, &namespace)
				assert.NilError(t, err)

				serviceAccount := corev1.ServiceAccount{
					TypeMeta: v1.TypeMeta{
						APIVersion: "",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: v1.ObjectMeta{
						Name:      "mysa",
						Namespace: "tenant",
					},
				}

				err = env.TestKubeClient.Create(env.Ctx, &serviceAccount)
				assert.NilError(t, err)

				role := rbacv1.ClusterRole{
					TypeMeta: v1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRole",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "imp",
					},
					Rules: []rbacv1.PolicyRule{
						{
							Verbs:     []string{"*"},
							Resources: []string{"*"},
							APIGroups: []string{"*"},
						},
					},
				}

				err = env.TestKubeClient.Create(env.Ctx, &role)
				assert.NilError(t, err)

				roleBinding := rbacv1.ClusterRoleBinding{
					TypeMeta: v1.TypeMeta{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
					},
					ObjectMeta: v1.ObjectMeta{
						Name: "imp",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "mysa",
							Namespace: "tenant",
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "imp",
					},
				}

				err = env.TestKubeClient.Create(env.Ctx, &roleBinding)
				assert.NilError(t, err)

				result, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)
				assert.Equal(t, result.Suspended, false)

				ctx := context.Background()
				ns := "monitoring"

				var dep appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "test", Namespace: ns},
					&dep,
				)
				assert.NilError(t, err)
				assert.Equal(t, dep.Name, "test")
				assert.Equal(t, dep.Namespace, ns)

				inventoryStorage, err := tcContext.inventoryInstance.Load()
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
			name: "WorkloadIdentity",
			prepare: func() *projecttest.Environment {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithProjectSource("workloadidentity"),
					projecttest.WithKubernetes(
						kubetest.WithVCSAuthSecretFor("workloadidentity"),
					),
				)
				err = helmtest.ReplaceTemplate(
					helmtest.Template{
						Name:                    "test",
						TestProjectPath:         env.Projects[0].TargetPath,
						RelativeReleaseFilePath: "infra/prometheus/releases.cue",
						RepoURL:                 azureHelmEnvironment.ChartServer.URL(),
					},
					env.Projects[0].GitRepository,
				)
				return &env
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject

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

				inventoryStorage, err := tcContext.inventoryInstance.Load()
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
			prepare: func() *projecttest.Environment {
				return nil
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject

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
		{
			name: "Conflicts",
			prepare: func() *projecttest.Environment {
				return nil
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject

				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)

				var deployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"},
					&deployment,
				)
				assert.NilError(t, err)

				var anotherDeployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "anothersubcomponent", Namespace: "prometheus"},
					&anotherDeployment,
				)
				assert.NilError(t, err)

				unstr := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "mysubcomponent",
							"namespace": "prometheus",
						},
						"spec": map[string]interface{}{
							"replicas": 2,
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"securityContext": map[string]interface{}{
										"runAsNonRoot":        false,
										"fsGroup":             0,
										"fsGroupChangePolicy": "Always",
									},
								},
							},
						},
					},
				}

				err := env.DynamicTestKubeClient.DynamicClient().Apply(
					env.Ctx,
					&unstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)

				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)

				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"},
					&deployment,
				)
				assert.NilError(t, err)
				assert.Equal(t, deployment.Name, "mysubcomponent")
				assert.Equal(t, deployment.Namespace, "prometheus")
				assert.Equal(t, *deployment.Spec.Replicas, int32(1))
				assert.Equal(
					t,
					*deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot,
					true,
				)
				assert.Equal(
					t,
					*deployment.Spec.Template.Spec.SecurityContext.FSGroup,
					int64(65532),
				)
				assert.Equal(
					t,
					*deployment.Spec.Template.Spec.SecurityContext.FSGroupChangePolicy,
					corev1.FSGroupChangeOnRootMismatch,
				)
			},
		},
		{
			name: "Ignore-Conflicts",
			prepare: func() *projecttest.Environment {
				return nil
			},
			run: func(t *testing.T, tcContext testCaseContext) {
				reconciler := tcContext.reconciler
				env := tcContext.environment
				gProject := tcContext.gitopsProject

				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)

				var deployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "mysubcomponent", Namespace: "prometheus"},
					&deployment,
				)
				assert.NilError(t, err)

				var anotherDeployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "anothersubcomponent", Namespace: "prometheus"},
					&anotherDeployment,
				)
				assert.NilError(t, err)

				anotherUnstr := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "anothersubcomponent",
							"namespace": "prometheus",
						},
						"spec": map[string]interface{}{
							"replicas": 2,
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"securityContext": map[string]interface{}{
										"runAsNonRoot":        false,
										"fsGroup":             0,
										"fsGroupChangePolicy": "Always",
									},
								},
							},
						},
					},
				}

				err = env.DynamicTestKubeClient.DynamicClient().Apply(
					env.Ctx,
					&anotherUnstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)

				_, err = reconciler.Reconcile(env.Ctx, gProject)
				assert.NilError(t, err)

				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Name: "anothersubcomponent", Namespace: "prometheus"},
					&anotherDeployment,
				)
				assert.NilError(t, err)
				assert.Equal(t, anotherDeployment.Name, "anothersubcomponent")
				assert.Equal(t, anotherDeployment.Namespace, "prometheus")
				assert.Equal(t, *anotherDeployment.Spec.Replicas, int32(2))
				assert.Equal(
					t,
					*anotherDeployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot,
					false,
				)
				assert.Equal(
					t,
					*anotherDeployment.Spec.Template.Spec.SecurityContext.FSGroup,
					int64(0),
				)
				assert.Equal(
					t,
					*anotherDeployment.Spec.Template.Spec.SecurityContext.FSGroupChangePolicy,
					corev1.FSGroupChangeOnRootMismatch,
				)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.prepare()
			if env == nil {
				defaultEnv := projecttest.StartProjectEnv(t,
					projecttest.WithProjectSource("simple"),
					projecttest.WithKubernetes(
						kubetest.WithVCSAuthSecretFor(strings.ToLower(tc.name)),
					),
				)
				env = &defaultEnv
				err = helmtest.ReplaceTemplate(
					helmtest.Template{
						Name:                    "test",
						TestProjectPath:         defaultEnv.Projects[0].TargetPath,
						RelativeReleaseFilePath: "infra/prometheus/releases.cue",
						RepoURL:                 publicHelmEnvironment.ChartServer.URL(),
					},
					defaultEnv.Projects[0].GitRepository,
				)
			}
			defer env.Stop()

			reconciler := project.Reconciler{
				KubeConfig:            env.ControlPlane.Config,
				RepositoryManager:     env.RepositoryManager,
				ProjectManager:        env.ProjectManager,
				Log:                   env.Log,
				FieldManager:          "controller",
				WorkerPoolSize:        runtime.GOMAXPROCS(0),
				InsecureSkipTLSverify: true,
			}

			testProject := env.Projects[0]

			suspend := false
			gProject := gitops.GitOpsProject{
				TypeMeta: v1.TypeMeta{
					APIVersion: "gitops.declcd.io/v1",
					Kind:       "GitOpsProject",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      tc.name,
					Namespace: "default",
					UID:       types.UID(env.TestRoot),
				},
				Spec: gitops.GitOpsProjectSpec{
					URL:                 testProject.TargetPath,
					Branch:              "main",
					PullIntervalSeconds: 5,
					Suspend:             &suspend,
				},
			}

			inventoryInstance := &inventory.Instance{
				// /inventory is mounted as volume.
				Path: filepath.Join("/inventory", string(gProject.GetUID())),
			}

			tc.run(t, testCaseContext{
				environment:       env,
				reconciler:        reconciler,
				gitopsProject:     gProject,
				inventoryInstance: inventoryInstance,
			})
		})
	}
}

var benchResult *project.ReconcileResult

func BenchmarkReconcile(b *testing.B) {
	fmt.Println("sizeof(kube.Manifest{}=)", unsafe.Sizeof(kube.Manifest{}))
	fmt.Println("sizeof(helm.Release{}=)", unsafe.Sizeof(helm.ReleaseComponent{}))

	var err error
	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	registryPath, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assertError(err)
	defer cueModuleRegistry.Close()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	// set to to true globally as CUE for example uses the DefaultTransport
	http.DefaultTransport = transport

	publicHelmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assertError(err)
	defer publicHelmEnvironment.Close()

	appTemplate := `
package {{ .Package }}

import (
	"github.com/kharf/declcd/schema/component"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
	appsv1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/apps/v1"
)

{{ .App }}Ns: component.#Manifest & {
	content: corev1.#Namespace & {
		apiVersion: string | *"v1"
		kind:       "Namespace"
		metadata: name: "{{ .App }}"
	}
}

{{ .App }}Deployment: component.#Manifest & {
	#Name:      "{{ .App }}"
	#Namespace: "{{ .App }}"
	dependencies: [
		{{ .App }}Ns.id
	]
	content: appsv1.#Deployment & {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      #Name
			namespace: #Namespace
			labels: app: #Name
		}
		spec: {
			replicas: 0
			selector: matchLabels: app: #Name
			template: {
				metadata: labels: app: #Name
				spec: containers: [{
					image: "test"
					name:  #Name
					ports: [{
						containerPort: 8080
					}]
				}]
			}
		}
	}
}	

{{ .App }}Release: component.#HelmRelease & {
	#Name:      "{{ .App }}"
	#Namespace: "{{ .App }}"
	dependencies: [
		{{ .App }}Ns.id
	]
	name: #Name
	namespace: #Namespace
	chart: {
		name: "test"
		repoURL: "{{ .RepoURL }}"
		version: "1.0.0"
	}
	values: {
		replicaCount: 0
		service: enabled: false
	}
}
`

	testProjectPath, err := os.MkdirTemp("", "")
	assert.NilError(b, err)
	defer os.RemoveAll(testProjectPath)

	appsDir := filepath.Join(testProjectPath, "apps")
	err = os.MkdirAll(appsDir, 0777)
	assert.NilError(b, err)

	cueModPath := filepath.Join(testProjectPath, "cue.mod")
	err = os.MkdirAll(cueModPath, 0777)
	assert.NilError(b, err)

	moduleFile := modfile.File{
		Module: "github.com/kharf/declcd/benchmark",
		Language: &modfile.Language{
			Version: "v0.9.2",
		},
		Deps: map[string]*modfile.Dep{
			"github.com/kharf/cuepkgs/modules/k8s@v0": {
				Version: "v0.0.5",
			},
			"github.com/kharf/declcd/schema@v0": {
				Version: "v0.9.1",
			},
		},
	}

	content, err := moduleFile.Format()
	assert.NilError(b, err)

	_, err = modfile.Parse(content, "module.cue")
	err = os.WriteFile(filepath.Join(cueModPath, "module.cue"), content, 0666)
	assert.NilError(b, err)

	for i := 0; i < 250; i++ {
		appName := fmt.Sprintf("app%v", i)
		file, err := os.Create(filepath.Join(appsDir, fmt.Sprintf("%s.cue", appName)))
		assert.NilError(b, err)
		tmpl, err := template.New("benchmark").Parse(appTemplate)
		assert.NilError(b, err)
		err = tmpl.Execute(file, map[string]any{
			"Package": "apps",
			"App":     appName,
			"RepoURL": publicHelmEnvironment.ChartServer.URL(),
		})
		assert.NilError(b, err)
	}

	logOpts := ctrlZap.Options{
		Development: false,
		Level:       zapcore.Level(0),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	repo, err := gittest.InitGitRepository(testProjectPath, "main")
	assert.NilError(b, err)
	defer repo.Clean()

	b.Run("Reconcile", func(b *testing.B) {
		maxProcs := runtime.GOMAXPROCS(0)
		b.ReportAllocs()
		b.ResetTimer()
		var result *project.ReconcileResult
		for n := 0; n < b.N; n++ {
			b.StopTimer()
			env := kubetest.StartKubetestEnv(b, log)

			projectManager := project.NewManager(component.NewBuilder(), log, maxProcs)
			reconciler := project.Reconciler{
				KubeConfig:            env.ControlPlane.Config,
				RepositoryManager:     env.RepositoryManager,
				ProjectManager:        projectManager,
				Log:                   log,
				FieldManager:          "controller",
				WorkerPoolSize:        maxProcs,
				InsecureSkipTLSverify: true,
			}

			suspend := false
			gProject := gitops.GitOpsProject{
				TypeMeta: v1.TypeMeta{
					APIVersion: "gitops.declcd.io/v1",
					Kind:       "GitOpsProject",
				},
				ObjectMeta: v1.ObjectMeta{
					Name:      "benchmark",
					Namespace: "default",
					UID: types.UID(
						fmt.Sprintf(
							"%v%v%v%v",
							testProjectPath,
							maxProcs,
							n,
							time.Now().Nanosecond(),
						),
					),
				},
				Spec: gitops.GitOpsProjectSpec{
					URL:                 testProjectPath,
					PullIntervalSeconds: 5,
					Suspend:             &suspend,
					Branch:              "main",
				},
			}

			b.StartTimer()
			result, _ = reconciler.Reconcile(env.Ctx, gProject)
			b.StopTimer()

			env.Stop()
		}

		benchResult = result
	})
}
