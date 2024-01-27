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
	"k8s.io/apimachinery/pkg/types"

	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	. "github.com/kharf/declcd/pkg/helm"
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
		post func(env projecttest.ProjectEnv, reconciler ChartReconciler)
	}{
		{
			name: "Default",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {},
		},
		{
			name: "OCI",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, true)))
				repoURL := strings.Replace(env.HelmEnv.ChartServer.URL, "http", "oci", 1)
				chart := Chart{
					Name:    "test",
					RepoURL: repoURL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {},
		},
		{
			name: "Namespaced",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "mynamespace",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {},
		},
		{
			name: "Cache",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: false,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
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
				err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: "default"}, &deployment)
				assert.Error(t, err, "deployments.apps \"test\" not found")
				fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}.testReconcile(t, reconciler, "test", "test", assertChartv1)
			},
		},
		{
			name: "Upgrade",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "mynamespace",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {
				releasesFilePath := filepath.Join(env.TestProject, "infra", "prometheus", "releases.cue")
				releasesContent, err := os.ReadFile(releasesFilePath)
				if err != nil {
					t.Fatal(err)
				}
				tmpl, err := template.New("releases").Parse(string(releasesContent))
				if err != nil {
					t.Fatal(err)
				}
				releasesFile, err := os.Create(filepath.Join(env.TestProject, "infra", "prometheus", "releases.cue"))
				if err != nil {
					t.Fatal(err)
				}
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
				if err != nil {
					t.Fatal(err)
				}
				err = env.GitRepository.CommitFile("infra/prometheus/releases.cue", "update chart to v2")
				if err != nil {
					t.Fatal(err)
				}
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "2.0.0",
				}
				fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "mynamespace",
					cleanChart: true,
				}.testReconcile(t, reconciler, "test", "test", assertChartv2)
			},
		},
		{
			name: "PendingUpgradeRecovery",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingUpgrade
				rel.Version = 2
				err = env.HelmEnv.HelmConfig.Releases.Create(rel)
				assert.NilError(t, err)
				fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}.testReconcile(t, reconciler, "test", "test", assertChartv1)
			},
		},
		{
			name: "PendingInstallRecovery",
			pre: func() (projecttest.ProjectEnv, fixture) {
				env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				return env, fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}
			},
			post: func(env projecttest.ProjectEnv, reconciler ChartReconciler) {
				chart := Chart{
					Name:    "test",
					RepoURL: env.HelmEnv.RepositoryServer.URL,
					Version: "1.0.0",
				}
				helmGet := action.NewGet(&env.HelmEnv.HelmConfig)
				rel, err := helmGet.Run("test")
				assert.NilError(t, err)
				rel.Info.Status = release.StatusPendingInstall
				err = env.HelmEnv.HelmConfig.Releases.Update(rel)
				assert.NilError(t, err)
				fixture{
					env:        env.KubetestEnv,
					chart:      chart,
					namespace:  "default",
					cleanChart: true,
				}.testReconcile(t, reconciler, "test", "test", assertChartv1)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env, fixture := tc.pre()
			defer env.Stop()
			reconciler := ChartReconciler{
				Cfg: env.HelmEnv.HelmConfig,
				Log: log,
			}
			fixture.testReconcile(t, reconciler, "test", "test", assertChartv1)
			tc.post(env, reconciler)
		})
	}
}

func assertChartv1(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &deployment)
	assert.NilError(t, err)
	assert.Equal(t, deployment.Name, liveName)
	assert.Equal(t, deployment.Namespace, namespace)
	var hpa autoscalingv2.HorizontalPodAutoscaler
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &hpa)
	assert.NilError(t, err)
	assert.Equal(t, hpa.Name, liveName)
	assert.Equal(t, hpa.Namespace, namespace)
	var svc corev1.Service
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &svc)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &svcAcc)
	assert.NilError(t, err)
	assert.Equal(t, svcAcc.Name, liveName)
	assert.Equal(t, svcAcc.Namespace, namespace)
}

func assertChartv2(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string) {
	ctx := context.Background()
	var deployment appsv1.Deployment
	err := env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &deployment)
	assert.NilError(t, err)
	assert.Equal(t, deployment.Name, liveName)
	assert.Equal(t, deployment.Namespace, namespace)
	var hpa autoscalingv2.HorizontalPodAutoscaler
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &hpa)
	assert.NilError(t, err)
	assert.Equal(t, hpa.Name, liveName)
	assert.Equal(t, hpa.Namespace, namespace)
	var svc corev1.Service
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &svc)
	assert.NilError(t, err)
	assert.Equal(t, svc.Name, liveName)
	assert.Equal(t, svc.Namespace, namespace)
	var svcAcc corev1.ServiceAccount
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: namespace}, &svcAcc)
	assert.Error(t, err, "serviceaccounts \"test\" not found")
}

type fixture struct {
	env        *kubetest.KubetestEnv
	chart      Chart
	namespace  string
	cleanChart bool
}

func (f fixture) testReconcile(
	t *testing.T,
	reconciler ChartReconciler,
	releaseName string,
	liveName string,
	assertion func(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string),
) {
	vals := Values{
		"autoscaling": map[string]interface{}{
			"enabled": true,
		},
	}
	if f.cleanChart {
		defer Remove(f.chart)
	}
	release, err := reconciler.Reconcile(
		f.chart,
		ReleaseName(releaseName),
		Namespace(f.namespace),
		vals,
	)
	assert.NilError(t, err)
	assert.Equal(t, release.Name, releaseName)
	assertion(t, f.env, liveName, f.namespace)
}
