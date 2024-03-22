package project

import (
	"os"
	"path/filepath"

	"cuelang.org/go/mod/modfile"
)

func Init(module string, path string) error {
	moduleDir := filepath.Join(path, "cue.mod")
	_, err := os.Stat(moduleDir)
	if err == nil {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	moduleFile := modfile.File{
		Module: module,
		Language: &modfile.Language{
			Version: "v0.8.0",
		},
		Deps: map[string]*modfile.Dep{
			"github.com/kharf/declcd/schema@v0": {
				Version: "v0.9.1",
			},
			"github.com/kharf/cuepkgs/modules/k8s@v0": {
				Version: "v0.0.5",
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
	return nil
}
