package core

import (
	"testing"
	"testing/fstest"

	"cuelang.org/go/cue/cuecontext"
	"gotest.tools/v3/assert"
)

func TestProjectManager_Load(t *testing.T) {
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
	}

	builder := NewFileEntryBuilder(ctx, mapfs, NewContentEntryBuilder(ctx))
	pm := NewProjectManager(mapfs, builder)
	proj, err := pm.Load("project/")
	assert.NilError(t, err)
	assert.Assert(t, len(proj.mainComponents) == 2)
	assert.Equal(t, proj.mainComponents[0].entry.Name, "apps")
	assert.Equal(t, proj.mainComponents[1].entry.Name, "infra")
}

func TestProjectManager_Load_AppsDoesNotExist(t *testing.T) {
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
	pm := NewProjectManager(mapfs, builder)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/apps/entry.cue")
}

func TestProjectManager_Load_InfraDoesNotExist(t *testing.T) {
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
	pm := NewProjectManager(mapfs, builder)
	_, err := pm.Load("project/")
	assert.ErrorIs(t, err, ErrMainComponentNotFound)
	assert.Error(t, err, "main component not found: could not load project/infra/entry.cue")
}