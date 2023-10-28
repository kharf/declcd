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

func TestChartReconciler_Reconcile_Default(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
	defer env.Stop()

	chart := Chart{
		Name:    "test",
		RepoURL: env.HelmEnv.RepositoryServer.URL,
		Version: "1.0.0",
	}

	reconciler := ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: log,
	}

	fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "default",
	}.testReconcile(t, "test", "test", true, assertChartv1)
}

func TestChartReconciler_Reconcile_Default_OCI(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, true)))
	defer env.Stop()

	repoURL := strings.Replace(env.HelmEnv.ChartServer.URL, "http", "oci", 1)
	chart := Chart{
		Name:    "test",
		RepoURL: repoURL,
		Version: "1.0.0",
	}

	reconciler := ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: log,
	}

	fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "default",
	}.testReconcile(t, "test", "test", true, assertChartv1)
}

func TestChartReconciler_Reconcile_Namespaced(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
	defer env.Stop()

	chart := Chart{
		Name:    "test",
		RepoURL: env.HelmEnv.RepositoryServer.URL,
		Version: "1.0.0",
	}

	reconciler := ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: log,
	}

	liveName := fmt.Sprintf("%s-%s", "myhelmrelease", "test")
	fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "mynamespace",
	}.testReconcile(t, "myhelmrelease", liveName, true, assertChartv1)
}

func TestChartReconciler_Reconcile_Upgrade(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
	defer env.Stop()

	chart := Chart{
		Name:    "test",
		RepoURL: env.HelmEnv.RepositoryServer.URL,
		Version: "1.0.0",
	}

	reconciler := ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: log,
	}

	liveName := fmt.Sprintf("%s-%s", "myhelmrelease", "test")
	fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "mynamespace",
	}.testReconcile(t, "myhelmrelease", liveName, true, assertChartv1)

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

	chart = Chart{
		Name:    "test",
		RepoURL: env.HelmEnv.RepositoryServer.URL,
		Version: "2.0.0",
	}

	fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "mynamespace",
	}.testReconcile(t, "myhelmrelease", liveName, true, assertChartv2)
}

func TestChartReconciler_Reconcile_Chart_Cache(t *testing.T) {
	env := projecttest.StartProjectEnv(t, projecttest.WithKubernetes(kubetest.WithHelm(true, false)))
	defer env.Stop()

	chart := Chart{
		Name:    "test",
		RepoURL: env.HelmEnv.RepositoryServer.URL,
		Version: "1.0.0",
	}

	reconciler := ChartReconciler{
		Cfg: env.HelmEnv.HelmConfig,
		Log: log,
	}

	liveName := fmt.Sprintf("%s-%s", "myhelmrelease", "test")
	fixture := fixture{
		env:        env.KubetestEnv,
		reconciler: reconciler,
		chart:      chart,
		namespace:  "mynamespace",
	}
	fixture.testReconcile(t, "myhelmrelease", liveName, false, assertChartv1)
	defer Remove(chart)
	env.HelmEnv.ChartServer.Close()
	env.HelmEnv.RepositoryServer.Close()
	ctx := context.Background()
	err := env.TestKubeClient.Delete(ctx, &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      liveName,
			Namespace: "mynamespace",
		},
	})
	assert.NilError(t, err)
	var deployment appsv1.Deployment
	err = env.TestKubeClient.Get(ctx, types.NamespacedName{Name: liveName, Namespace: "mynamespace"}, &deployment)
	assert.Error(t, err, "deployments.apps \"myhelmrelease-test\" not found")
	fixture.testReconcile(t, "myhelmrelease", liveName, true, assertChartv1)
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
	assert.Error(t, err, "serviceaccounts \"myhelmrelease-test\" not found")
}

type fixture struct {
	env        *kubetest.KubetestEnv
	reconciler ChartReconciler
	chart      Chart
	namespace  string
}

func (f fixture) testReconcile(
	t *testing.T,
	releaseName string,
	liveName string,
	cleanChart bool,
	assertion func(t *testing.T, env *kubetest.KubetestEnv, liveName string, namespace string),
) {
	vals := Values{
		"autoscaling": map[string]interface{}{
			"enabled": true,
		},
	}

	if cleanChart {
		defer Remove(f.chart)
	}

	release, err := f.reconciler.Reconcile(
		f.chart,
		ReleaseName(releaseName),
		Namespace(f.namespace),
		vals,
	)

	assert.NilError(t, err)
	assert.Equal(t, release.Name, releaseName)

	assertion(t, f.env, liveName, f.namespace)
}
