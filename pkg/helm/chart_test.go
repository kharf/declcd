package helm_test

import (
	"context"
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

func TestChartReconciler_Reconcile(t *testing.T) {
	testCases := []struct {
		name string
		pre  func() (projecttest.ProjectEnv, fixture)
		post func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture)
	}{
		{
			name: "Default",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				vals := Values{
					"autoscaling": map[string]interface{}{
						"enabled": true,
					},
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    vals,
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
				var hpa autoscalingv2.HorizontalPodAutoscaler
				err := env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{
						Name:      fixture.expectedReleaseName,
						Namespace: fixture.expectedNamespace,
					},
					&hpa,
				)
				assert.NilError(t, err)
				assert.Equal(t, hpa.Name, fixture.expectedReleaseName)
				assert.Equal(t, hpa.Namespace, fixture.expectedNamespace)
			},
		},
		{
			name: "OCI",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, true)),
				)
				repoURL := strings.Replace(env.HelmEnv.ChartServer.URL, "http", "oci", 1)
				chart := Chart{
					Name:    "test",
					RepoURL: repoURL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {},
		},
		{
			name: "Namespaced",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "mynamespace",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "mynamespace",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {},
		},
		{
			name: "Cache",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          false,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
				env.HelmEnv.ChartServer.Close()
				env.HelmEnv.RepositoryServer.Close()
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
				fixture.cleanChart = true
				fixture.expectedVersion = 2
				fixture.testReconcile(t, reconciler, assertChartv1)
			},
		},
		{
			name: "Upgrade",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
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
					RepoUrl: env.HelmEnv.RepositoryServer.URL,
					Version: "2.0.0",
				})
				assert.NilError(t, err)
				err = env.GitRepository.CommitFile(
					"infra/prometheus/releases.cue",
					"update chart to v2",
				)
				assert.NilError(t, err)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "2.0.0",
				}
				fixture.cleanChart = true
				fixture.release.Chart = chart
				fixture.expectedVersion = 2
				fixture.testReconcile(t, reconciler, assertChartv2)
			},
		},
		{
			name: "NoUpgrade",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
				fixture.testReconcile(t, reconciler, assertChartv1)
			},
		},
		{
			name: "Conflict",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
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
				fixture.expectedVersion = 2
				fixture.testReconcile(t, reconciler, assertChartv1)
			},
		},
		{
			name: "PendingUpgradeRecovery",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingUpgrade
				rel.Version = 2
				err = env.HelmEnv.HelmConfig.Releases.Create(rel)
				assert.NilError(t, err)
				fixture.expectedVersion = 2
				fixture.testReconcile(t, reconciler, assertChartv1)
			},
		},
		{
			name: "PendingInstallRecovery",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(
					t,
					projecttest.WithKubernetes(kubetest.WithHelm(true, false)),
				)
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env: env.KubetestEnv,
					release: helm.ReleaseDeclaration{
						Name:      "test",
						Namespace: "",
						Chart:     chart,
						Values:    Values{},
					},
					expectedReleaseName: "test",
					expectedNamespace:   "default",
					expectedVersion:     1,
					cleanChart:          true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler, fixture fixture) {
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingInstall
				err = env.HelmEnv.HelmConfig.Releases.Update(rel)
				assert.NilError(t, err)
				fixture.testReconcile(t, reconciler, assertChartv1)
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env, fixture := tc.pre()
			defer env.Stop()
			reconciler := NewChartReconciler(
				env.ControlPlane.Config,
				env.DynamicTestKubeClient,
				"controller",
				env.InventoryManager,
				env.Log,
			)
			fixture.testReconcile(t, reconciler, assertChartv1)
			tc.post(env, reconciler, fixture)
		})
	}
}

func assertChartv1(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string) {
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

func assertChartv2(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string) {
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

type fixture struct {
	env                 *kubetest.KubetestEnv
	release             helm.ReleaseDeclaration
	expectedReleaseName string
	expectedNamespace   string
	expectedVersion     int
	cleanChart          bool
}

func (f fixture) testReconcile(
	t *testing.T,
	reconciler ChartReconciler,
	assertion func(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string),
) {
	if f.cleanChart {
		defer Remove(f.release.Chart)
	}
	release, err := reconciler.Reconcile(
		f.env.Ctx,
		"test",
		f.release,
	)
	assert.NilError(t, err)
	assert.Equal(t, release.Version, f.expectedVersion)
	assert.Equal(t, release.Name, f.expectedReleaseName)
	assert.Equal(t, release.Namespace, f.expectedNamespace)
	assertion(t, f.env, f.expectedReleaseName, f.expectedNamespace)
}
