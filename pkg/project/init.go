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

package project

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"

	"cuelang.org/go/mod/modfile"
	"github.com/kharf/declcd/internal/manifest"
)

const (
	ControllerNamespace = "declcd-system"
	ControllerName      = "gitops-controller"
)

func Init(module string, path string, version string) error {
	moduleDir := filepath.Join(path, "cue.mod")
	_, err := os.Stat(moduleDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		moduleFile := modfile.File{
			Module: module,
			Language: &modfile.Language{
				Version: "v0.9.2",
			},
			Deps: map[string]*modfile.Dep{
				"github.com/kharf/declcd/schema@v0": {
					Version: "v" + version,
				},
			},
		}
		content, err := moduleFile.Format()
		if err != nil {
			return err
		}
		if _, err := modfile.Parse(content, "module.cue"); err != nil {
			return err
		}
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "module.cue"), content, 0666); err != nil {
			return err
		}
	}
	declcdDir := filepath.Join(path, "declcd")
	_, err = os.Stat(declcdDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		tmpl, err := template.New("").Parse(manifest.System)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{
			"Name":    ControllerName,
			"Version": version,
		}); err != nil {
			return err
		}
		if err := os.MkdirAll(declcdDir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(declcdDir, "system.cue"), buf.Bytes(), 0666); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(declcdDir, "crd.cue"), []byte(manifest.CRD), 0666); err != nil {
			return err
		}
	}
	return nil
}
