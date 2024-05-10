// Copyright 2024 Google LLC
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
	"strings"
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

	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/helm"
	. "github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	log logr.Logger
)

func TestMain(m *testing.M) {
	opts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log = ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	os.Exit(m.Run())
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

func TestChartReconciler_Reconcile(t *testing.T) {
	testCases := []struct {
		name string
		pre  func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc)
		post func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration)
	}{
		{
			name: "HTTP",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{
						"autoscaling": map[string]interface{}{
							"enabled": true,
						},
					})
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				var hpa autoscalingv2.HorizontalPodAutoscaler
				err := env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{
						Name:      releaseDeclaration.Name,
						Namespace: releaseDeclaration.Namespace,
					},
					&hpa,
				)
				assert.NilError(t, err)
				assert.Equal(t, hpa.Name, releaseDeclaration.Name)
				assert.Equal(t, hpa.Namespace, releaseDeclaration.Namespace)
			},
		},
		{
			name: "HTTP-AuthSecretNotFound",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)
				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					&Auth{
						SecretRef: SecretRef{
							Name:      "repauth",
							Namespace: "default",
						},
					},
					Values{},
				)
				return env, release, func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
					assert.Error(t, reconcileErr, "secrets \"repauth\" not found")
				}
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "HTTP-Auth",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, true)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					applyRepoAuthSecret(t, env, false),
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "OCI",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, true, false)),
				)

				release := createReleaseDeclaration(
					"default",
					replaceOCIHost(env),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "OCI-AuthSecretNotFound",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, true, false)),
				)

				release := createReleaseDeclaration(
					"default",
					replaceOCIHost(env),
					"1.0.0",
					&Auth{
						SecretRef: SecretRef{
							Name:      "regauth",
							Namespace: "default",
						},
					},
					Values{},
				)
				return env, release, func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
					assert.Error(t, reconcileErr, "secrets \"regauth\" not found")
				}
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "OCI-AuthSecretEmptyHost",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, true, true)),
				)

				release := createReleaseDeclaration(
					"default",
					replaceOCIHost(env),
					"1.0.0",
					applyRepoAuthSecret(t, env, false),
					Values{},
				)
				return env, release, func(t *testing.T, env *kubetest.Environment, reconcileErr error, actualRelease *helm.Release, liveName, namespace string) {
					assert.Error(t, reconcileErr, "Auth secret value not found: host is empty")
				}
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "OCI-Auth",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, true, true)),
				)

				release := createReleaseDeclaration(
					"default",
					replaceOCIHost(env),
					"1.0.0",
					applyRepoAuthSecret(t, env, true),
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "Namespaced",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"mynamespace",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
			},
		},
		{
			name: "Cached",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				env.HelmEnv.ChartServer.Close()
				ctx := context.Background()
				err := env.TestKubeClient.Delete(ctx, &appsv1.Deployment{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
				})
				assert.NilError(t, err)
				var deployment appsv1.Deployment
				err = env.TestKubeClient.Get(
					ctx,
					types.NamespacedName{Name: "test", Namespace: "default"},
					&deployment,
				)
				assert.Error(t, err, "deployments.apps \"test\" not found")
				actualRelease, err := reconciler.Reconcile(
					ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv1(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "Upgrade",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				releasesFilePath := filepath.Join(
					env.TestProject,
					"infra",
					"prometheus",
					"releases.cue",
				)
				releasesContent, err := os.ReadFile(releasesFilePath)
				assert.NilError(t, err)
				tmpl, err := template.New("releases").Parse(string(releasesContent))
				assert.NilError(t, err)
				releasesFile, err := os.Create(
					filepath.Join(env.TestProject, "infra", "prometheus", "releases.cue"),
				)
				assert.NilError(t, err)
				defer releasesFile.Close()
				err = tmpl.Execute(releasesFile, struct {
					Name    string
					RepoUrl string
					Version string
				}{
					Name:    "test",
					RepoUrl: env.HelmEnv.ChartServer.URL(),
					Version: "2.0.0",
				})
				assert.NilError(t, err)
				_, err = env.GitRepository.CommitFile(
					"infra/prometheus/releases.cue",
					"update chart to v2",
				)
				assert.NilError(t, err)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.ChartServer.URL(),
					Version: "2.0.0",
				}
				releaseDeclaration.Chart = chart
				actualRelease, err := reconciler.Reconcile(
					env.Ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv2(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "NoUpgrade",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)
				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				actualRelease, err := reconciler.Reconcile(
					env.Ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv1(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 1)
			},
		},
		{
			name: "Conflict",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
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
				err := env.DynamicTestKubeClient.Apply(
					env.Ctx,
					&unstr,
					"imposter",
					kube.Force(true),
				)
				assert.NilError(t, err)
				actualRelease, err := reconciler.Reconcile(
					env.Ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv1(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "PendingUpgradeRecovery",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingUpgrade
				rel.Version = 2
				err = env.HelmEnv.HelmConfig.Releases.Create(rel)
				assert.NilError(t, err)
				actualRelease, err := reconciler.Reconcile(
					env.Ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv1(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 2)
			},
		},
		{
			name: "PendingInstallRecovery",
			pre: func() (projecttest.Environment, helm.ReleaseDeclaration, assertFunc) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false, false)),
				)

				release := createReleaseDeclaration(
					"default",
					env.HelmEnv.ChartServer.URL(),
					"1.0.0",
					nil,
					Values{},
				)
				return env, release, defaultAssertionFunc(release)
			},
			post: func(env projecttest.Environment, reconciler ChartReconciler, releaseDeclaration helm.ReleaseDeclaration) {
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingInstall
				err = env.HelmEnv.HelmConfig.Releases.Update(rel)
				assert.NilError(t, err)
				actualRelease, err := reconciler.Reconcile(
					env.Ctx,
					releaseDeclaration,
					fmt.Sprintf(
						"%s_%s_%s",
						releaseDeclaration.Name,
						releaseDeclaration.Namespace,
						"HelmRelease",
					),
				)
				assert.NilError(t, err)
				assertChartv1(t, env.Environment, actualRelease.Name, actualRelease.Namespace)
				assert.Equal(t, actualRelease.Version, 1)
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env, releaseDeclaration, assertFunc := tc.pre()
			defer env.Stop()
			err := Remove(releaseDeclaration.Chart)
			assert.NilError(t, err)
			chartReconciler := helm.ChartReconciler{
				KubeConfig:            env.ControlPlane.Config,
				Client:                env.DynamicTestKubeClient,
				FieldManager:          "controller",
				InventoryManager:      env.InventoryManager,
				InsecureSkipTLSverify: true,
				Log:                   env.Log,
			}
			release, err := chartReconciler.Reconcile(
				env.Ctx,
				releaseDeclaration,
				fmt.Sprintf(
					"%s_%s_%s",
					releaseDeclaration.Name,
					releaseDeclaration.Namespace,
					"HelmRelease",
				),
			)
			assertFunc(
				t,
				env.Environment,
				err,
				release,
				releaseDeclaration.Name,
				releaseDeclaration.Namespace,
			)
			tc.post(env, chartReconciler, releaseDeclaration)
			err = Remove(releaseDeclaration.Chart)
			assert.NilError(t, err)
		})
	}
}

// has to be declcd.io, because of Docker defaulting to HTTP, when localhost is detected.
// more info in internal/kubetest/env.go#startHelmServer
func replaceOCIHost(env projecttest.Environment) string {
	repoURL := strings.Replace(
		env.HelmEnv.ChartServer.URL(),
		"https://127.0.0.1",
		"oci://declcd.io",
		1,
	)
	return repoURL
}

func applyRepoAuthSecret(t *testing.T, env projecttest.Environment, withHost bool) *Auth {
	name := "repauth"
	namespace := "default"
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
	if withHost {
		data := unstr.Object["data"].(map[string][]byte)
		data["host"] = []byte(env.HelmEnv.ChartServer.URL())
	}
	err := env.DynamicTestKubeClient.Apply(
		env.Ctx,
		&unstr,
		"charttest",
	)
	assert.NilError(t, err)
	return &Auth{
		SecretRef: SecretRef{
			Name:      name,
			Namespace: namespace,
		},
	}
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
