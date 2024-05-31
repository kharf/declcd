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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/go-logr/logr"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/zap/zapcore"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

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
	"github.com/kharf/declcd/pkg/kube"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log                       logr.Logger
	publicHelmEnvironment     *helmtest.Environment
	privateHelmEnvironment    *helmtest.Environment
	publicOciHelmEnvironment  *helmtest.Environment
	privateOciHelmEnvironment *helmtest.Environment
	gcpHelmEnvironment        *helmtest.Environment
	azureHelmEnvironment      *helmtest.Environment
	awsEnvironment            *cloudtest.AWSEnvironment
)

func TestMain(m *testing.M) {
	opts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log = ctrlZap.New(ctrlZap.UseFlagOptions(&opts))

	testRoot, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	cueModuleRegistry, err := ocitest.StartCUERegistry(testRoot)
	assertError(err)
	defer cueModuleRegistry.Close()

	publicHelmEnvironment = newHelmEnvironment(false, false, "")
	defer publicHelmEnvironment.Close()

	privateHelmEnvironment = newHelmEnvironment(false, true, "")
	defer privateHelmEnvironment.Close()

	publicOciHelmEnvironment = newHelmEnvironment(true, false, "")
	defer publicOciHelmEnvironment.Close()

	privateOciHelmEnvironment = newHelmEnvironment(true, true, "")
	defer privateOciHelmEnvironment.Close()

	gcpHelmEnvironment = newHelmEnvironment(true, true, cloud.GCP)
	defer gcpHelmEnvironment.Close()
	gcpCloudEnvironment, err := cloudtest.NewGCPEnvironment()
	assertError(err)
	defer gcpCloudEnvironment.Close()

	azureHelmEnvironment = newHelmEnvironment(true, true, cloud.Azure)
	defer azureHelmEnvironment.Close()
	azureCloudEnvironment, err := cloudtest.NewAzureEnvironment()
	assertError(err)
	defer azureCloudEnvironment.Close()

	awsHelmEnvironment := newHelmEnvironment(true, true, cloud.AWS)
	defer awsHelmEnvironment.Close()
	awsEnvironment, err = cloudtest.NewAWSEnvironment(
		awsHelmEnvironment.ChartServer.Addr(),
	)
	assertError(err)
	defer awsEnvironment.Close()

	cloudEnvironment, err := cloudtest.NewMetaServer(
		azureCloudEnvironment.OIDCIssuerServer.URL,
	)
	assertError(err)
	defer cloudEnvironment.Close()

	os.Exit(m.Run())
}

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

type assertFunc func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName string, namespace string)

func defaultAssertionFunc(release ReleaseDeclaration) assertFunc {
	return func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
		assert.NilError(t, reconcileErr)
		assertChartv1(t, env, liveName, namespace)
		assert.Equal(t, actualRelease.Version, 1)
		assert.Equal(t, actualRelease.Name, release.Name)
		assert.Equal(t, actualRelease.Namespace, release.Namespace)
	}
}

type testCaseContext struct {
	environment        *projecttest.Environment
	chartServer        helmtest.Server
	releaseDeclaration helm.ReleaseDeclaration
	createAuthSecret   bool
	assertFunc         assertFunc
	chartReconciler    helm.ChartReconciler
}

func TestChartReconciler_Reconcile(t *testing.T) {
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
					Values{
						"autoscaling": map[string]interface{}{
							"enabled": true,
						},
					})

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
					Values{},
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
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
					Values{},
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
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
					Values{},
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
					Values{},
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
					Values{},
				)

				return testCaseContext{
					releaseDeclaration: release,
					createAuthSecret:   false,
					chartServer:        privateHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
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
					Values{},
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
					Values{},
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc: func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
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
					Values{},
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
					Values{},
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
					Values{},
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
					Values{},
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
					Values{},
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
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
				)
				assert.Equal(t, actualRelease.Version, 2)
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
					Values{},
				)

				return testCaseContext{
					releaseDeclaration: release,
					chartServer:        publicHelmEnvironment.ChartServer,
					assertFunc:         defaultAssertionFunc(release),
				}
			},
			postRun: func(context testCaseContext) {
				releasesFilePath := filepath.Join(
					context.environment.TestProject,
					"infra",
					"prometheus",
					"releases.cue",
				)
				releasesContent, err := os.ReadFile(releasesFilePath)
				assert.NilError(t, err)

				tmpl, err := template.New("releases").Parse(string(releasesContent))
				assert.NilError(t, err)

				releasesFile, err := os.Create(
					filepath.Join(
						context.environment.TestProject,
						"infra",
						"prometheus",
						"releases.cue",
					),
				)
				assert.NilError(t, err)
				defer releasesFile.Close()

				err = tmpl.Execute(releasesFile, struct {
					Name    string
					RepoUrl string
					Version string
				}{
					Name:    "test",
					RepoUrl: context.chartServer.URL(),
					Version: "2.0.0",
				})
				assert.NilError(t, err)

				_, err = context.environment.GitRepository.CommitFile(
					"infra/prometheus/releases.cue",
					"update chart to v2",
				)
				assert.NilError(t, err)

				chart := Chart{
					Name:    "test",
					RepoURL: context.chartServer.URL(),
					Version: "2.0.0",
				}

				context.releaseDeclaration.Chart = chart
				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
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
					Values{},
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
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
				)
				assert.Equal(t, actualRelease.Version, 1)
			},
		},
		{
			name: "Conflict",
			setup: func() testCaseContext {
				release := createReleaseDeclaration(
					"default",
					publicHelmEnvironment.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
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

				err := context.environment.DynamicTestKubeClient.Apply(
					context.environment.Ctx,
					&unstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)

				actualRelease, err := context.chartReconciler.Reconcile(
					context.environment.Ctx,
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
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
					Values{},
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
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
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
					Values{},
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
					context.releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						context.releaseDeclaration.Name,
						context.releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)

				assertChartv1(
					t,
					context.environment.Environment,
					actualRelease.Name,
					actualRelease.Namespace,
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

			err = helmtest.ReplaceTemplate(
				context.environment.TestProject,
				context.environment.GitRepository,
				context.chartServer.URL(),
			)
			assert.NilError(t, err)

			chartReconciler := helm.ChartReconciler{
				KubeConfig:            context.environment.ControlPlane.Config,
				Client:                context.environment.DynamicTestKubeClient,
				FieldManager:          "controller",
				InventoryManager:      context.environment.InventoryManager,
				InsecureSkipTLSverify: true,
				Log:                   context.environment.Log,
			}
			context.chartReconciler = chartReconciler

			release, err := chartReconciler.Reconcile(
				context.environment.Ctx,
				context.releaseDeclaration,
				fmt.Sprintf(
					"%s_%s_%s",
					context.releaseDeclaration.Name,
					context.releaseDeclaration.Namespace,
					"HelmRelease",
				),
			)

			context.assertFunc(
				t,
				context.environment.Environment,
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
	err := env.DynamicTestKubeClient.Apply(
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
	values Values,
) ReleaseDeclaration {
	release := helm.ReleaseDeclaration{
		Name:      "test",
		Namespace: namespace,
		Chart: Chart{
			Name:    "test",
			RepoURL: url,
			Version: "1.0.0",
			Auth:    auth,
		},
		Values: values,
	}
	return release
}

func assertChartv1(t *testing.T, env *kubetest.Environment, liveName string, namespace string) {
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
	assert.NilError(t, err)
	assert.Equal(t, svcAcc.Name, liveName)
	assert.Equal(t, svcAcc.Namespace, namespace)
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
}
