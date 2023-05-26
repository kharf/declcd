package core

import (
	"os"
	"testing"
	"testing/fstest"

	"cuelang.org/go/cue/cuecontext"
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
	ctx := cuecontext.New()
	mapfs := fstest.MapFS{
		"project/apps/entry.cue": {
			Data: []byte(`
		entry: apps: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/entry.cue": {
			Data: []byte(`
		entry: infra: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/xxxx.cue": {
			Data: []byte(`
		entry: xxxx: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/entry.cue": {
			Data: []byte(`
		entry: "node-exporter": {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/deployment.cue": {
			Data: []byte(`
		node-exporter: deployment: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/plugin/entry.cue": {
			Data: []byte(`
		entry: "plugin": {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/node-exporter/plugin/deployment.cue": {
			Data: []byte(`
		plugin: deployment: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/entry.cue": {
			Data: []byte(`
		entry: prometheus: {
		 intervalSeconds: 60
		}
	`),
		},
		"project/infra/prometheus/deployment.cue": {
			Data: []byte(`
		prometheus: deployment: {
		 intervalSeconds: 60
		}
	`),
		},
	}

	builder := NewFileEntryBuilder(ctx, mapfs, NewContentEntryBuilder(ctx))
	pm := NewProjectManager(mapfs, builder, logger)
	proj, err := pm.Load("project/")
	assert.NilError(t, err)
	assert.Assert(t, len(proj.MainComponents) == 2)
	apps := proj.MainComponents[0]
	assert.Equal(t, apps.entry.Name, "apps")
	assert.Assert(t, len(apps.SubComponents) == 0)
	infra := proj.MainComponents[1]
	assert.Equal(t, infra.entry.Name, "infra")
	assert.Assert(t, len(infra.SubComponents) == 1)
	prometheus := infra.SubComponents[0]
	assert.Equal(t, prometheus.Entry.Name, "prometheus")
	assert.Assert(t, len(prometheus.Manifests) == 1)
	assert.Equal(t, prometheus.Manifests[0].name, "deployment.cue")
	assert.Assert(t, len(prometheus.SubComponents) == 1)
	nodeExporter := prometheus.SubComponents[0]
	assert.Equal(t, nodeExporter.Entry.Name, "node-exporter")
	assert.Assert(t, len(nodeExporter.SubComponents) == 1)
	assert.Assert(t, len(nodeExporter.Manifests) == 1)
	assert.Equal(t, nodeExporter.Manifests[0].name, "deployment.cue")
	plugin := nodeExporter.SubComponents[0]
	assert.Equal(t, plugin.Entry.Name, "plugin")
	assert.Assert(t, len(plugin.SubComponents) == 0)
	assert.Assert(t, len(plugin.Manifests) == 1)
	assert.Equal(t, plugin.Manifests[0].name, "deployment.cue")
}

func TestProjectManager_Load_AppsDoesNotExist(t *testing.T) {
	logger := setUp(t)
	ctx := cuecontext.New()
	mapfs := fstest.MapFS{
		"project/infra/entry.cue": {
			Data: []byte(`
		entry: infra: {
		 intervalSeconds: 60
		}
	`),
		},
	}

	builder := NewFileEntryBuilder(ctx, mapfs, NewContentEntryBuilder(ctx))
	pm := NewProjectManager(mapfs, builder, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/apps/entry.cue")
}

func TestProjectManager_Load_InfraDoesNotExist(t *testing.T) {
	logger := setUp(t)
	ctx := cuecontext.New()
	mapfs := fstest.MapFS{
		"project/apps/entry.cue": {
			Data: []byte(`
		entry: apps: {
		 intervalSeconds: 60
		}
	`),
		},
	}

	builder := NewFileEntryBuilder(ctx, mapfs, NewContentEntryBuilder(ctx))
	pm := NewProjectManager(mapfs, builder, logger)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/infra/entry.cue")
}

func TestProjectManager_Load_TestData(t *testing.T) {
	logger := setUp(t)
	ctx := cuecontext.New()
	fileSystem := os.DirFS("testdata")
	builder := NewFileEntryBuilder(ctx, fileSystem, NewContentEntryBuilder(ctx))
	pm := NewProjectManager(fileSystem, builder, logger)
	proj, err := pm.Load("mib")
	assert.NilError(t, err)
	assert.Assert(t, len(proj.MainComponents) == 2)
	apps := proj.MainComponents[0]
	assert.Equal(t, apps.entry.Name, "apps")
	assert.Assert(t, len(apps.SubComponents) == 0)
	infra := proj.MainComponents[1]
	assert.Equal(t, infra.entry.Name, "infra")
	assert.Assert(t, len(infra.SubComponents) == 1)
	prometheus := infra.SubComponents[0]
	assert.Equal(t, prometheus.Entry.Name, "prometheus")
	assert.Assert(t, len(prometheus.Manifests) == 2)
	assert.Equal(t, prometheus.Manifests[0].name, "deployment.cue")
	assert.Equal(t, prometheus.Manifests[1].name, "namespace.cue")
	assert.Assert(t, len(prometheus.SubComponents) == 1)
}
