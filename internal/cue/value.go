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
	cuecontext.EvaluatorVersion(cuecontext.EvalV3)
	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return nil, value.Err()
	}

	if err := value.Validate(); err != nil {
		return nil, err
	}
	return &value, nil
}
