package cue

import (
	"fmt"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

func BuildPackage(
	packagePath string,
	projectRoot string,
) (*cue.Value, error) {
	harmonizedPackagePath := packagePath
	currentDirectoryPrefix := "./"
	if !strings.HasPrefix(packagePath, currentDirectoryPrefix) {
		harmonizedPackagePath = currentDirectoryPrefix + packagePath
	}
	cfg := &load.Config{
		Package:    filepath.Base(harmonizedPackagePath),
		ModuleRoot: projectRoot,
		Dir:        projectRoot,
	}
	instances := load.Instances([]string{harmonizedPackagePath}, cfg)
	if len(instances) > 1 {
		return nil, fmt.Errorf(
			"too many cue instances found. Make sure to only use a single cue package inside a directory",
		)
	}
	instance := instances[0]
	if instance.Err != nil {
		return nil, instance.Err
	}
	ctx := cuecontext.New()
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return nil, value.Err()
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	return &value, nil
}
