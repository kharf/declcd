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

package helm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/kharf/declcd/test/workingdir"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/kharf/declcd/internal/cloudtest"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/helm"
	. "github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
)

func newHelmEnvironment(
	oci bool,
	private bool,
	cloudProvider cloud.ProviderID,
) *helmtest.Environment {
	helmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(oci),
		helmtest.WithPrivate(private),
		helmtest.WithProvider(cloudProvider),
	)
	assertError(err)
	return helmEnvironment
}

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type assertFunc func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName string, namespace string)

func defaultAssertionFunc(release *ReleaseDeclaration) assertFunc {
	return func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
		assert.NilError(t, reconcileErr)
		assertChartv1(t, env, liveName, namespace, 1)
		assert.Equal(t, actualRelease.Version, 1)
		assert.Equal(t, actualRelease.Name, release.Name)
		assert.Equal(t, actualRelease.Namespace, release.Namespace)

		contentReader, err := inventoryInstance.GetItem(&inventory.HelmReleaseItem{
			Name:      release.Name,
			Namespace: release.Namespace,
			ID:        fmt.Sprintf("%s_%s_HelmRelease", release.Name, release.Namespace),
		})
		defer contentReader.Close()

		storedBytes, err := io.ReadAll(contentReader)
		assert.NilError(t, err)

		desiredBuf := &bytes.Buffer{}
		err = json.NewEncoder(desiredBuf).Encode(release)
		assert.NilError(t, err)

		assert.Equal(t, string(storedBytes), desiredBuf.String())
	}
}

func defaultPatchesAssertionFunc(release *ReleaseDeclaration) assertFunc {
	return func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
		assert.NilError(t, reconcileErr)
		assertChartv1Patches(t, env, liveName, namespace)
		assert.Equal(t, actualRelease.Version, 1)
		assert.Equal(t, actualRelease.Name, release.Name)
		assert.Equal(t, actualRelease.Namespace, release.Namespace)

		contentReader, err := inventoryInstance.GetItem(&inventory.HelmReleaseItem{
			Name:      release.Name,
			Namespace: release.Namespace,
			ID:        fmt.Sprintf("%s_%s_HelmRelease", release.Name, release.Namespace),
		})
		defer contentReader.Close()

		storedBytes, err := io.ReadAll(contentReader)
		assert.NilError(t, err)

		desiredBuf := &bytes.Buffer{}
		err = json.NewEncoder(desiredBuf).Encode(release)
		assert.NilError(t, err)

		assert.Equal(t, string(storedBytes), desiredBuf.String())
	}
}

func defaultV2AssertionFunc(release *ReleaseDeclaration) assertFunc {
	return func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
		assert.NilError(t, reconcileErr)
		assertChartv2(t, env, liveName, namespace)
		assert.Equal(t, actualRelease.Version, 1)
		assert.Equal(t, actualRelease.Name, release.Name)
		assert.Equal(t, actualRelease.Namespace, release.Namespace)

		contentReader, err := inventoryInstance.GetItem(&inventory.HelmReleaseItem{
			Name:      release.Name,
			Namespace: release.Namespace,
			ID:        fmt.Sprintf("%s_%s_HelmRelease", release.Name, release.Namespace),
		})
		defer contentReader.Close()

		storedRelease := helm.Release{}
		err = json.NewDecoder(contentReader).Decode(&storedRelease)
		assert.NilError(t, err)
		assert.DeepEqual(t, storedRelease, release)
	}
}

type testCaseContext struct {
	environment        *projecttest.Environment
	chartServer        helmtest.Server
	releaseDeclaration *helm.ReleaseDeclaration
	createAuthSecret   bool
	assertFunc         assertFunc
	chartReconciler    helm.ChartReconciler
}

func TestChartReconciler_Reconcile(t *testing.T) {
	testRoot, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	cueModuleRegistry, err := ocitest.StartCUERegistry(testRoot)
	assertError(err)
	defer cueModuleRegistry.Close()

	publicHelmEnvironment := newHelmEnvironment(false, false, "")
	defer publicHelmEnvironment.Close()

	privateHelmEnvironment := newHelmEnvironment(false, true, "")
	defer privateHelmEnvironment.Close()

	publicOciHelmEnvironment := newHelmEnvironment(true, false, "")
	defer publicOciHelmEnvironment.Close()

	privateOciHelmEnvironment := newHelmEnvironment(true, true, "")
	defer privateOciHelmEnvironment.Close()

	gcpHelmEnvironment := newHelmEnvironment(true, true, cloud.GCP)
	defer gcpHelmEnvironment.Close()
	gcpCloudEnvironment, err := cloudtest.NewGCPEnvironment()
	assertError(err)
	defer gcpCloudEnvironment.Close()

	azureHelmEnvironment := newHelmEnvironment(true, true, cloud.Azure)
	defer azureHelmEnvironment.Close()
	azureCloudEnvironment, err := cloudtest.NewAzureEnvironment()
	assertError(err)
	defer azureCloudEnvironment.Close()

	awsHelmEnvironment := newHelmEnvironment(true, true, cloud.AWS)
	defer awsHelmEnvironment.Close()
	awsEnvironment, err := cloudtest.NewAWSEnvironment(
		awsHelmEnvironment.ChartServer.Addr(),
	)
	assertError(err)
	defer awsEnvironment.Close()

	cloudEnvironment, err := cloudtest.NewMetaServer(
		azureCloudEnvironment.OIDCIssuerServer.URL,
	)
	assertError(err)
	defer cloudEnvironment.Close()

	testCases := []struct {
		name    string
		setup   func() testCaseContext
		postRun func(context testCaseContext)
	}{
		{
			name: "HTTP",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{
						"autoscaling": map[string]interface{}{
							"enabled": true,
						},
					},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				var hpa autoscalingv2.HorizontalPodAutoscaler
				err := context.environment.TestKubeClient.Get(
					context.environment.Ctx,
					types.NamespacedName{
						Name:      context.releaseDeclaration.Name,
						Namespace: context.releaseDeclaration.Namespace,
					},
					&hpa,
				)
				assert.NilError(t, err)
				assert.Equal(t, hpa.Name, context.releaseDeclaration.Name)
				assert.Equal(t, hpa.Namespace, context.releaseDeclaration.Namespace)
			},
		},
		{
			name: "HTTP-Auth-Secret-Not-Found",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: &SecretRef{
							Name:      "repauth",
							Namespace: "default",
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
						assert.Error(t, reconcileErr, "secrets \"repauth\" not found")
					},
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "HTTP-Auth-Secret-SecretRef-Not-Set",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: nil,
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
						assert.ErrorIs(t, reconcileErr, helm.ErrAuthSecretValueNotFound)
					},
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "HTTP-Auth",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					privateHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: &SecretRef{
							Name:      "auth",
							Namespace: "default",
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					createAuthSecret:   true,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicOciHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicOciHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-Auth-Secret-Not-Found",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					privateOciHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: &SecretRef{
							Name:      "regauth",
							Namespace: "default",
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					createAuthSecret:   false,
					chartServer:        privateHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
						assert.Error(t, reconcileErr, "secrets \"regauth\" not found")
					},
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-Secret-Auth",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					privateOciHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: &SecretRef{
							Name:      "auth",
							Namespace: "default",
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					createAuthSecret:   true,
					chartServer:        privateOciHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-Secret-Auth-SecretRef-Not-Set",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					privateOciHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: nil,
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, inventoryInstance inventory.Instance, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
						assert.ErrorIs(t, reconcileErr, helm.ErrAuthSecretValueNotFound)
					},
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-GCP-Workload-Identity-Auth",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					gcpHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						WorkloadIdentity: &WorkloadIdentity{
							Provider: string(cloud.GCP),
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-AWS-Workload-Identity-Auth",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					awsEnvironment.ECRServer.URL,
					"1.0.0",
					&Auth{
						WorkloadIdentity: &WorkloadIdentity{
							Provider: string(cloud.AWS),
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "OCI-Azure-Workload-Identity-Auth",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					azureHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					&Auth{
						WorkloadIdentity: &WorkloadIdentity{
							Provider: string(cloud.Azure),
						},
					},
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "Namespaced",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"mynamespace",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "Cached",
			setup: func() testCaseContext {
				flakyHelmEnvironment, err := helmtest.NewHelmEnvironment(
					helmtest.WithOCI(false),
					helmtest.WithPrivate(false),
				)
				assert.NilError(t, err)

				release := createReleaseDeclaration(
					"default",
					flakyHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        flakyHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				context.chartServer.Close()
				ctx := context.environment.Ctx
				err := context.environment.TestKubeClient.Delete(ctx, &appsv1.Deployment{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
				})
				assert.NilError(t, err)

				var deployment appsv1.Deployment
				err = context.environment.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "test", Namespace: "default"},
					&deployment,
				)
				assert.Error(t, err, "deployments.apps \"test\" not found")

				actualRelease, err := context.chartReconciler.Reconcile(
					ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					1,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Install-Patches",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					&helm.Patches{
						Unstructureds: map[string]kube.ExtendedUnstructured{
							"v1-Service-default-test": {
								Unstructured: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"apiVersion": "v1",
										"kind":       "Service",
										"metadata": map[string]any{
											"name":      "test",
											"namespace": "default",
										},
										"spec": map[string]any{
											"type": "NodePort",
										},
									},
								},
							},
							"apps/v1-Deployment-default-test": {
								Unstructured: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"apiVersion": "apps/v1",
										"kind":       "Deployment",
										"metadata": map[string]any{
											"name": "test",
										},
										"spec": map[string]any{
											"replicas": int64(2),
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
					})

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultPatchesAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
			},
		},
		{
			name: "Upgrade",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				chart := Chart{
					Name:    "test",
					RepoURL: context.chartServer.URL(),
					Version: "2.0.0",
				}

				context.releaseDeclaration.Chart = chart
				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv2(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Upgrade-CRDs",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"2.0.0",
					nil,
					true,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultV2AssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				chart := Chart{
					Name:    "test",
					RepoURL: context.chartServer.URL(),
					Version: "3.0.0",
				}

				context.releaseDeclaration.Chart = chart
				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv3(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "No-Allowance-To-Upgrade-CRDs",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"2.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultV2AssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				chart := Chart{
					Name:    "test",
					RepoURL: context.chartServer.URL(),
					Version: "3.0.0",
				}

				context.releaseDeclaration.Chart = chart
				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv2(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "No-Upgrade",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					true,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					1,
				)
				assert.Equal(t, actualRelease.Version, 1)
			},
		},
		{
			name: "Conflicts",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				unstr := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"replicas": 2,
						},
					},
				}

				err := context.environment.DynamicTestKubeClient.DynamicClient().Apply(
					context.environment.Ctx,
					&unstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)

				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					1,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Ignore-Conflicts",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					&helm.Patches{
						Unstructureds: map[string]kube.ExtendedUnstructured{
							"apps/v1-Deployment-default-test": {
								Unstructured: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"apiVersion": "apps/v1",
										"kind":       "Deployment",
										"metadata": map[string]any{
											"name":      "test",
											"namespace": "default",
										},
										"spec": map[string]any{
											"replicas": int64(1),
										},
									},
								},
								Metadata: &kube.ManifestMetadataNode{
									"spec": &kube.ManifestMetadataNode{
										"replicas": &kube.ManifestFieldMetadata{
											IgnoreAttr: kube.OnConflict,
										},
									},
								},
							},
						},
					},
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				unstr := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"replicas": 2,
						},
					},
				}

				err := context.environment.DynamicTestKubeClient.DynamicClient().Apply(
					context.environment.Ctx,
					&unstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)

				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					2,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Pending-Upgrade-Recovery",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				helmConfig, err := helmtest.ConfigureHelm(context.chartReconciler.KubeConfig)
				assert.NilError(t, err)

				helmGet := action.NewGet(helmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)

				rel.Info.Status = release.StatusPendingUpgrade
				rel.Version = 2

				err = helmConfig.Releases.Create(rel)
				assert.NilError(t, err)

				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					1,
				)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Pending-Install-Recovery",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					false,
					Values{},
					nil,
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				helmConfig, err := helmtest.ConfigureHelm(context.chartReconciler.KubeConfig)
				assert.NilError(t, err)

				helmGet := action.NewGet(helmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)

				rel.Info.Status = release.StatusPendingInstall
				err = helmConfig.Releases.Update(rel)
				assert.NilError(t, err)

				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					&helm.ReleaseComponent{
						ID: fmt.Sprintf(
							"%s_%s_%s",
							context.releaseDeclaration.Name,
							context.releaseDeclaration.Namespace,
							"HelmRelease",
						),
						Content: context.releaseDeclaration,
					},
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
					1,
				)
				assert.Equal(t, actualRelease.Version, 1)
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			context := tc.setup()
			if context.environment == nil {
				env := projecttest.StartProjectEnv(
					t,
				)
				defer env.Stop()
				context.environment = &env
			}

			inventoryInstance := inventory.Instance{
				Path: filepath.Join(context.environment.TestRoot, "inventory"),
			}

			auth := context.releaseDeclaration.Chart.Auth
			if auth != nil && auth.SecretRef != nil && context.createAuthSecret {
				applyRepoAuthSecret(
					t,
					auth.SecretRef.Name,
					auth.SecretRef.Namespace,
					context.environment,
				)
			}

			err := Remove(context.releaseDeclaration.Chart)
			defer Remove(context.releaseDeclaration.Chart)
			assert.NilError(t, err)

			chartReconciler := helm.ChartReconciler{
				Log:                   context.environment.Log,
				KubeConfig:            context.environment.ControlPlane.Config,
				Client:                context.environment.DynamicTestKubeClient,
				FieldManager:          "controller",
				InventoryInstance:     &inventoryInstance,
				InsecureSkipTLSverify: true,
			}
			context.chartReconciler = chartReconciler

			ns := &unstructured.Unstructured{}
			ns.SetAPIVersion("v1")
			ns.SetKind("Namespace")
			ns.SetName(context.releaseDeclaration.Namespace)

			err = context.environment.DynamicTestKubeClient.DynamicClient().Apply(
				context.environment.Ctx,
				ns,
				"controller",
			)
			assert.NilError(t, err)

			release, err := chartReconciler.Reconcile(
				context.environment.Ctx,
				&helm.ReleaseComponent{
					ID: fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
					Content: context.releaseDeclaration,
				},
			)

			context.assertFunc(
				t,
				context.environment.Environment,
				inventoryInstance,
				err,
				release,
				context.releaseDeclaration.Name,
				context.releaseDeclaration.Namespace,
			)

			tc.postRun(context)
		})
	}
}

func applyRepoAuthSecret(
	t *testing.T,
	name string,
	namespace string,
	env *projecttest.Environment,
) {
	unstr := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"data": map[string][]byte{
				"username": []byte("declcd"),
				"password": []byte("abcd"),
			},
		},
	}
	err := env.DynamicTestKubeClient.DynamicClient().Apply(
		env.Ctx,
		&unstr,
		"charttest",
	)
	assert.NilError(t, err)
}

func createReleaseDeclaration(
	namespace string,
	url string,
	version string,
	auth *Auth,
	allowUpgrade bool,
	values Values,
	patches *Patches,
) *ReleaseDeclaration {
	release := helm.ReleaseDeclaration{
		Name:      "test",
		Namespace: namespace,
		CRDs: CRDs{
			AllowUpgrade: allowUpgrade,
		},
		Chart: Chart{
			Name:    "test",
			RepoURL: url,
			Version: version,
			Auth:    auth,
		},
		Values:  values,
		Patches: patches,
	}
	return &release
}

func assertChartv1(
	t *testing.T,
	env *kubetest.Environment,
	liveName string,
	namespace string,
	replicas int32,
) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&deployment,
	)
	assert.NilError(t, err)

	gracePeriodSeconds := int64(30)
	historyLimit := int32(10)
	progressDeadlineSeconds := int32(600)
	expectedDeployment := appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance":   "test",
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/name":       "test",
				"app.kubernetes.io/version":    "1.16.0",
				"helm.sh/chart":                "test-1.0.0",
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      "test",
				"meta.helm.sh/release-namespace": namespace,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": "test",
					"app.kubernetes.io/name":     "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/instance": "test",
						"app.kubernetes.io/name":     "test",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "nginx:1.16.0",
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.IntOrString{
											Type:   intstr.String,
											StrVal: "http",
										},
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.IntOrString{
											Type:   intstr.String,
											StrVal: "http",
										},
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: int32(80),
									Protocol:      "TCP",
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "IfNotPresent",
							SecurityContext:          &corev1.SecurityContext{},
						},
					},
					SecurityContext:               &corev1.PodSecurityContext{},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: &gracePeriodSeconds,
					DNSPolicy:                     "ClusterFirst",
					ServiceAccountName:            "test",
					DeprecatedServiceAccount:      "test",
					SchedulerName:                 "default-scheduler",
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			MinReadySeconds:         0,
			RevisionHistoryLimit:    &historyLimit,
			Paused:                  false,
			ProgressDeadlineSeconds: &progressDeadlineSeconds,
		},
	}

	EqualDeployment(t, deployment, expectedDeployment)

	var svc corev1.Service
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	assert.Equal(t, svc.Spec.Type, corev1.ServiceTypeClusterIP)

	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svcAcc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svcAcc.Name, liveName)
	assert.Equal(t, svcAcc.Namespace, namespace)
	assertCRDNoChanges(t, ctx, env.DynamicTestKubeClient.DynamicClient())
}

func assertChartv1Patches(
	t *testing.T,
	env *kubetest.Environment,
	liveName string,
	namespace string,
) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&deployment,
	)
	assert.NilError(t, err)

	expectedReplicas := int32(2)
	gracePeriodSeconds := int64(30)
	historyLimit := int32(10)
	progressDeadlineSeconds := int32(600)
	expectedDeployment := appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/instance":   "test",
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/name":       "test",
				"app.kubernetes.io/version":    "1.16.0",
				"helm.sh/chart":                "test-1.0.0",
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      "test",
				"meta.helm.sh/release-namespace": "default",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &expectedReplicas,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": "test",
					"app.kubernetes.io/name":     "test",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/instance": "test",
						"app.kubernetes.io/name":     "test",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "prometheus",
							Image: "prometheus:1.14.2",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(80),
									Protocol:      "TCP",
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "IfNotPresent",
						},
						{
							Name:  "sidecar",
							Image: "sidecar:1.14.2",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: int32(80),
									Protocol:      "TCP",
								},
							},
							TerminationMessagePath:   "/dev/termination-log",
							TerminationMessagePolicy: "File",
							ImagePullPolicy:          "IfNotPresent",
						},
					},
					SecurityContext:               &corev1.PodSecurityContext{},
					RestartPolicy:                 corev1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: &gracePeriodSeconds,
					DNSPolicy:                     "ClusterFirst",
					ServiceAccountName:            "test",
					DeprecatedServiceAccount:      "test",
					SchedulerName:                 "default-scheduler",
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			MinReadySeconds:         0,
			RevisionHistoryLimit:    &historyLimit,
			Paused:                  false,
			ProgressDeadlineSeconds: &progressDeadlineSeconds,
		},
	}

	EqualDeployment(t, deployment, expectedDeployment)

	var svc corev1.Service
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	assert.Equal(t, svc.Spec.Type, corev1.ServiceTypeNodePort)

	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svcAcc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svcAcc.Name, liveName)
	assert.Equal(t, svcAcc.Namespace, namespace)
	assertCRDNoChanges(t, ctx, env.DynamicTestKubeClient.DynamicClient())
}

func EqualDeployment(
	t *testing.T,
	actual appsv1.Deployment,
	expected appsv1.Deployment,
) {
	actual.UID = ""
	actual.ResourceVersion = ""
	actual.Generation = 0
	actual.CreationTimestamp = v1.Time{}
	actual.ManagedFields = nil

	assert.DeepEqual(t, actual, expected)
}

func assertCRDNoChanges(t *testing.T, ctx context.Context, dynamicClient *kube.DynamicClient) {
	crontabCRD := &unstructured.Unstructured{}
	crontabCRDName := "crontabs.stable.example.com"
	crontabCRD.SetName(crontabCRDName)
	crontabCRD.SetAPIVersion("apiextensions.k8s.io/v1")
	crontabCRD.SetKind("CustomResourceDefinition")
	crontabCRD, err := dynamicClient.Get(ctx, crontabCRD)
	assert.NilError(t, err)

	replicas, _ := getReplicas(crontabCRD)
	propType, _ := replicas["type"].(string)
	assert.Equal(t, propType, "integer")
}

func getReplicas(crontabCRD *unstructured.Unstructured) (map[string]interface{}, bool) {
	spec, _ := crontabCRD.Object["spec"].(map[string]interface{})
	versions, _ := spec["versions"].([]interface{})
	version, _ := versions[0].(map[string]interface{})
	schema, _ := version["schema"].(map[string]interface{})
	openAPISchema, _ := schema["openAPIV3Schema"].(map[string]interface{})
	properties, _ := openAPISchema["properties"].(map[string]interface{})
	propSpec, _ := properties["spec"].(map[string]interface{})
	specProperties, _ := propSpec["properties"].(map[string]interface{})
	replicas, ok := specProperties["replicas"].(map[string]interface{})
	return replicas, ok
}

func assertChartv2(t *testing.T, env *kubetest.Environment, liveName string, namespace string) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&deployment,
	)
	assert.NilError(t, err)
	assert.Equal(t, deployment.Name, liveName)
	assert.Equal(t, deployment.Namespace, namespace)
	var hpa autoscalingv2.HorizontalPodAutoscaler
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&hpa,
	)
	assert.Error(t, err, "horizontalpodautoscalers.autoscaling \"test\" not found")
	var svc corev1.Service
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svcAcc,
	)
	assert.Error(t, err, "serviceaccounts \"test\" not found")
	assertCRDNoChanges(t, ctx, env.DynamicTestKubeClient.DynamicClient())
}

func assertChartv3(t *testing.T, env *kubetest.Environment, liveName string, namespace string) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&deployment,
	)
	assert.NilError(t, err)
	assert.Equal(t, deployment.Name, liveName)
	assert.Equal(t, deployment.Namespace, namespace)
	var hpa autoscalingv2.HorizontalPodAutoscaler
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&hpa,
	)
	assert.Error(t, err, "horizontalpodautoscalers.autoscaling \"test\" not found")
	var svc corev1.Service
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svc,
	)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: liveName, Namespace: namespace},
		&svcAcc,
	)
	assert.Error(t, err, "serviceaccounts \"test\" not found")
	assertCRDChartv3(t, ctx, env.DynamicTestKubeClient.DynamicClient())
}

func assertCRDChartv3(t *testing.T, ctx context.Context, dynamicClient *kube.DynamicClient) {
	crontabCRD := &unstructured.Unstructured{}
	crontabCRDName := "crontabs.stable.example.com"
	crontabCRD.SetName(crontabCRDName)
	crontabCRD.SetAPIVersion("apiextensions.k8s.io/v1")
	crontabCRD.SetKind("CustomResourceDefinition")
	crontabCRD, err := dynamicClient.Get(ctx, crontabCRD)
	assert.NilError(t, err)

	_, ok := getReplicas(crontabCRD)
	assert.Assert(t, !ok)
}
