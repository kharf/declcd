package project

import (
	"os"
	"testing"
	"testing/fstest"

	"go.uber.org/zap"
	"gotest.tools/v3/assert"
)

func initZap() (*zap.Logger, error) {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	return zapConfig.Build()
}

func setUp(t *testing.T) *zap.SugaredLogger {
	l, err := initZap()
	if err != nil {
		t.Error(err)
	}
	return l.Sugar()
}

func TestProjectManager_Load(t *testing.T) {
	logger := setUp(t)
	mapfs := fstest.MapFS{
		"project/infra/xxxx.cue": {
			Data: []byte(`
		xxxx: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/apps": {},
		"project/infra/prometheus/node-exporter/component.cue": {
			Data: []byte(`
		component: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/deployment.cue": {
			Data: []byte(`
		_deployment: {
		 kind: Deployment
		}
	`),
		},
		"project/infra/prometheus/node-exporter/plugin/component.cue": {
			Data: []byte(`
		component: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/plugin/deployment.cue": {
			Data: []byte(`
		deployment: {
		 kind: Deployment
		}
	`),
		},
		"project/infra/prometheus/component.cue": {
			Data: []byte(`
		component: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/deployment.cue": {
			Data: []byte(`
		deployment: {
		 kind: Deployment
		}
	`),
		},
	}

	pm := NewProjectManager(mapfs, logger)
	mainComponents, err := pm.Load("project/")
	assert.NilError(t, err)
	assert.Assert(t, len(mainComponents) == 2)
	apps := mainComponents[0]
	assert.Assert(t, len(apps.SubComponents) == 0)
	infra := mainComponents[1]
	assert.Assert(t, len(infra.SubComponents) == 1)
	prometheus := infra.SubComponents[0]
	assert.Assert(t, len(prometheus.SubComponents) == 1)
	nodeExporter := prometheus.SubComponents[0]
	assert.Assert(t, len(nodeExporter.SubComponents) == 1)
	plugin := nodeExporter.SubComponents[0]
	assert.Assert(t, len(plugin.SubComponents) == 0)
}

func TestProjectManager_Load_AppsDoesNotExist(t *testing.T) {
	logger := setUp(t)
	mapfs := fstest.MapFS{
		"project/infra": {},
	}

	pm := NewProjectManager(mapfs, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/apps")
}

func TestProjectManager_Load_InfraDoesNotExist(t *testing.T) {
	logger := setUp(t)
	mapfs := fstest.MapFS{
		"project/apps/": {},
	}

	pm := NewProjectManager(mapfs, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/infra")
}

func TestProjectManager_Load_TestData(t *testing.T) {
	logger := setUp(t)
	fileSystem := os.DirFS("testdata")
	pm := NewProjectManager(fileSystem, logger)
	mainComponents, err := pm.Load("simple")
	assert.NilError(t, err)
	assert.Assert(t, len(mainComponents) == 2)
	apps := mainComponents[0]
	assert.Assert(t, len(apps.SubComponents) == 0)
	infra := mainComponents[1]
	assert.Assert(t, len(infra.SubComponents) == 1)
	prometheus := infra.SubComponents[0]
	assert.Assert(t, len(prometheus.SubComponents) == 1)
}
