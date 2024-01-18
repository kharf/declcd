package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func setUp(t *testing.T) logr.Logger {
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
	logger := setUp(t)
	cwd, err := os.Getwd()
	assert.NilError(t, err)
	root := filepath.Join(cwd, "test", "testdata", "simple")
	pm := project.NewManager(component.NewBuilder(), logger)
	dag, err := pm.Load(root)
	assert.NilError(t, err)
	linkerd := dag.Get("linkerd")
	assert.Equal(t, linkerd.Path(), "infra/linkerd")
	prometheus := dag.Get("prometheus")
	assert.Equal(t, prometheus.Path(), "infra/prometheus")
	subcomponent := dag.Get("subcomponent")
	assert.Equal(t, subcomponent.Path(), "infra/prometheus/subcomponent")
}
