package core

import (
	"testing"
	"testing/fstest"

	"cuelang.org/go/cue/cuecontext"
	"gotest.tools/v3/assert"
)

func TestContentEntryBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewContentEntryBuilder(ctx)
	result, err := builder.Build(`
		entry: app: {
		 intervalSeconds: 1
		}
	`,
	)
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "app")
	assert.Equal(t, result.IntervalSeconds, 1)

	result, err = builder.Build(`
		entry: infrastructure: {
		 intervalSeconds: 60
		 dependencies: ["app"]
		}
	`,
	)
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "infrastructure")
	assert.Equal(t, result.IntervalSeconds, 60)
	assert.Assert(t, len(result.Dependencies) == 1)
	assert.Equal(t, result.Dependencies[0], "app")
}

func TestContentEntryBuilder_Build_Schema(t *testing.T) {
	ctx := cuecontext.New()
	builder := NewContentEntryBuilder(ctx)
	_, err := builder.Build(`
		entry: app: {
		 intervalSeconds: "60"
		}
	`,
	)
	assert.Error(t, err, "entry.app.intervalSeconds: 2 errors in empty disjunction: (and 2 more errors)")

	_, err = builder.Build(`
		entry: app: {
		 intervalSeconds: 60
		 dependencies: [1]
		}
	`,
	)
	assert.Error(t, err, "entry.app.dependencies.0: conflicting values 1 and string (mismatched types int and string)")
}

func TestFileEntryBuilder_Build(t *testing.T) {
	ctx := cuecontext.New()
	fs := fstest.MapFS{
		"entry.cue": {
			Data: []byte(`
		entry: app: {
		 intervalSeconds: 60
		}
	`),
		},
	}
	builder := NewFileEntryBuilder(ctx, fs, NewContentEntryBuilder(ctx))
	result, err := builder.Build("entry.cue")
	assert.NilError(t, err)
	assert.Equal(t, result.Name, "app")
	assert.Equal(t, result.IntervalSeconds, 60)
}
