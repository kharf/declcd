package project_test

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/go-logr/logr"
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

func TestProjectManager_Load_AppsDoesNotExist(t *testing.T) {
	logger := setUp(t)
	mapfs := fstest.MapFS{
		"project/infra": {},
	}

	pm := project.NewProjectManager(project.FileSystem{FS: mapfs, Root: ""}, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, project.ErrMainComponentNotFound)
	assert.Error(t, err, "Main component not found: Could not load project/apps")
}

func TestProjectManager_Load_InfraDoesNotExist(t *testing.T) {
	logger := setUp(t)
	mapfs := fstest.MapFS{
		"project/apps/": {},
	}

	pm := project.NewProjectManager(project.FileSystem{FS: mapfs, Root: ""}, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, project.ErrMainComponentNotFound)
	assert.Error(t, err, "Main component not found: Could not load project/infra")
}

func TestProjectManager_Load(t *testing.T) {
	logger := setUp(t)
	root := "test/testdata"
	fileSystem := os.DirFS(root)
	pm := project.NewProjectManager(project.FileSystem{FS: fileSystem, Root: root}, logger)
	mainComponents, err := pm.Load("simple")
	assert.NilError(t, err)
	assert.Assert(t, len(mainComponents) == 2)
	apps := mainComponents[1]
	assert.Assert(t, len(apps.SubComponents) == 0)
	infra := mainComponents[0]
	assert.Assert(t, len(infra.SubComponents) == 1)
	prometheus := infra.SubComponents[0]
	assert.Equal(t, prometheus.Path, "infra/prometheus")
	assert.Assert(t, len(prometheus.SubComponents) == 1)
	subcomponent := prometheus.SubComponents[0]
	assert.Equal(t, subcomponent.Path, "infra/prometheus/subcomponent")
	assert.Assert(t, len(subcomponent.SubComponents) == 0)
}
