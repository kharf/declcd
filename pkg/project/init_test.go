// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
		name          string
		run           func() string
		expectedFiles []string
		assert        func(path string, expectedFiles []string)
	}{
		{
			name: "Primary",
			run: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				err = project.Init(
					"github.com/kharf/declcd/init@v0",
					"primary",
					false,
					path,
					"0.1.0",
				)
				assert.NilError(t, err)
				return path
			},
			expectedFiles: []string{
				"declcd/primary.cue",
				"declcd/primary_system.cue",
				"declcd/crd.cue",
			},
			assert: func(path string, expectedFiles []string) {
				assertModule(t, path, "github.com/kharf/declcd/init@v0", expectedFiles)
			},
		},
		{
			name: "Secondary",
			run: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				err = project.Init(
					"github.com/kharf/declcd/init@v0",
					"primary",
					false,
					path,
					"0.1.0",
				)
				assert.NilError(t, err)
				err = project.Init(
					"github.com/kharf/declcd/init@v0",
					"secondary",
					true,
					path,
					"0.1.0",
				)
				assert.NilError(t, err)
				return path
			},
			expectedFiles: []string{
				"declcd/primary.cue",
				"declcd/primary_system.cue",
				"declcd/crd.cue",
				"declcd/secondary_system.cue",
			},
			assert: func(path string, expectedFiles []string) {
				assertModule(t, path, "github.com/kharf/declcd/init@v0", expectedFiles)
			},
		},
		{
			name: "Exists",
			run: func() string {
				path, err := os.MkdirTemp("", "")
				assert.NilError(t, err)
				moduleFile := modfile.File{
					Module: "mymodule@v0",
					Language: &modfile.Language{
						Version: "v0.8.0",
					},
					Deps: map[string]*modfile.Dep{
						"github.com/kharf/declcd/schema@v0": {
							Version: "v0.1.0",
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
				err = project.Init(
					"github.com/kharf/declcd/init@v0",
					"primary",
					false,
					path,
					"0.1.0",
				)
				assert.NilError(t, err)
				return path
			},
			expectedFiles: []string{
				"declcd/primary.cue",
				"declcd/primary_system.cue",
				"declcd/crd.cue",
			},
			assert: func(path string, expectedFiles []string) {
				assertModule(t, path, "mymodule@v0", expectedFiles)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.run()
			defer os.RemoveAll(path)
			tc.assert(path, tc.expectedFiles)
		})
	}
}

func assertModule(t *testing.T, path string, module string, expectedFiles []string) {
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
		Version: "v0.1.0",
	})
	for _, file := range expectedFiles {
		_, err = os.Open(filepath.Join(path, file))
		assert.NilError(t, err)

	}
}
