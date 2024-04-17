package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/mod/modfile"
	"github.com/kharf/declcd/pkg/project"
	"gotest.tools/v3/assert"
)

func TestInit(t *testing.T) {
	testCases := []struct {
		name        string
		expectedErr string
		module      string
		pre         func() string
		assert      func(path string)
	}{
		{
			name:        "Success",
			expectedErr: "",
			module:      "github.com/kharf/declcd/init@v0",
			pre: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				return path
			},
			assert: func(path string) {
				assertModule(t, path, "github.com/kharf/declcd/init@v0")
			},
		},
		{
			name:        "Exists",
			expectedErr: "",
			module:      "github.com/kharf/declcd/init@v0",
			pre: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				moduleFile := modfile.File{
					Module: "mymodule@v0",
					Language: &modfile.Language{
						Version: "v0.8.0",
					},
					Deps: map[string]*modfile.Dep{
						"github.com/kharf/declcd/schema@v0": {
							Version: "v0.9.1",
						},
					},
				}
				content, err := moduleFile.Format()
				assert.NilError(t, err)
				moduleDir := filepath.Join(path, "cue.mod")
				err = os.MkdirAll(moduleDir, 0755)
				assert.NilError(t, err)
				err = os.WriteFile(filepath.Join(moduleDir, "module.cue"), content, 0666)
				assert.NilError(t, err)
				return path
			},
			assert: func(path string) {
				assertModule(t, path, "mymodule@v0")
			},
		},
		{
			name:        "WrongModuleFormat",
			expectedErr: "module path \"github.com/kharf/declcd/init\" in module.cue does not contain major version",
			module:      "github.com/kharf/declcd/init",
			pre: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				return path
			},
			assert: func(path string) {
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.pre()
			err := project.Init(tc.module, path)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				tc.assert(path)
			}
		})
	}
}

func assertModule(t *testing.T, path string, module string) {
	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Assert(t, info.IsDir())
	moduleDir := filepath.Join(path, "cue.mod")
	info, err = os.Stat(moduleDir)
	assert.NilError(t, err)
	assert.Assert(t, info.IsDir())
	moduleFilePath := filepath.Join(moduleDir, "module.cue")
	info, err = os.Stat(moduleFilePath)
	assert.NilError(t, err)
	assert.Assert(t, !info.IsDir())
	content, err := os.ReadFile(moduleFilePath)
	assert.NilError(t, err)
	moduleFile, err := modfile.Parse(content, "module.cue")
	assert.NilError(t, err)
	assert.Equal(t, moduleFile.Module, module)
	assert.Assert(t, strings.HasPrefix(moduleFile.Language.Version, "v"))
	assert.Assert(t, len(moduleFile.Deps) == 1)
	schemaModule := moduleFile.Deps["github.com/kharf/declcd/schema@v0"]
	assert.Equal(t, *schemaModule, modfile.Dep{
		Version: "v0.9.1",
	})
	declcdSystemFiles := []string{
		"declcd/system.cue",
		"declcd/crd.cue",
	}
	for _, file := range declcdSystemFiles {
		_, err = os.Open(filepath.Join(path, file))
		assert.NilError(t, err)

	}
}
