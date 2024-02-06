package project_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
	)
}

func setUp() logr.Logger {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logOpts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	return log
}

func TestManager_Load(t *testing.T) {
	logger := setUp()
	cwd, err := os.Getwd()
	assert.NilError(t, err)
	root := filepath.Join(cwd, "test", "testdata", "simple")
	pm := project.NewManager(component.NewBuilder(), logger, runtime.GOMAXPROCS(0))
	dag, err := pm.Load(root)
	assert.NilError(t, err)
	linkerd := dag.Get("linkerd")
	assert.Equal(t, linkerd.Path(), "infra/linkerd")
	prometheus := dag.Get("prometheus")
	assert.Equal(t, prometheus.Path(), "infra/prometheus")
	subcomponent := dag.Get("subcomponent")
	assert.Equal(t, subcomponent.Path(), "infra/prometheus/subcomponent")
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
