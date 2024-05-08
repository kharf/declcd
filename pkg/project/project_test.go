package project_test

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func setUp() logr.Logger {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logOpts := ctrlZap.Options{
		DestWriter:  io.Discard,
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	return log
}

func TestManager_Load(t *testing.T) {
	defer goleak.VerifyNone(
		t,
	)
	env := projecttest.StartProjectEnv(t,
		projecttest.WithKubernetes(
			kubetest.WithHelm(false, false, false),
		),
	)
	defer env.Stop()
	helmtest.ReplaceTemplate(
		t,
		env.TestProject,
		env.GitRepository,
		"oci://empty",
	)
	logger := setUp()
	root := env.TestProject
	pm := project.NewManager(component.NewBuilder(), logger, runtime.GOMAXPROCS(0))
	dag, err := pm.Load(root)
	assert.NilError(t, err)
	linkerd := dag.Get("linkerd___Namespace")
	assert.Assert(t, linkerd != nil)
	linkerdManifest, ok := linkerd.(*component.Manifest)
	assert.Assert(t, ok)
	assert.Assert(t, linkerdManifest.Content.GetAPIVersion() == "v1")
	assert.Assert(t, linkerdManifest.Content.GetKind() == "Namespace")
	assert.Assert(t, linkerdManifest.Content.GetName() == "linkerd")
	prometheus := dag.Get("prometheus___Namespace")
	assert.Assert(t, prometheus != nil)
	prometheusRelease := dag.Get("test_prometheus_HelmRelease")
	assert.Assert(t, prometheusRelease != nil)
	subcomponent := dag.Get("mysubcomponent_prometheus_apps_Deployment")
	assert.Assert(t, subcomponent != nil)
}

var dagResult *component.DependencyGraph

func BenchmarkManager_Load(b *testing.B) {
	b.ReportAllocs()
	logger := setUp()
	cwd, err := os.Getwd()
	assert.NilError(b, err)
	root := filepath.Join(cwd, "test", "testdata", "complex")
	pm := project.NewManager(component.NewBuilder(), logger, runtime.GOMAXPROCS(0))
	b.ResetTimer()
	var dag *component.DependencyGraph
	for n := 0; n < b.N; n++ {
		dag, err = pm.Load(root)
	}
	dagResult = dag
}
