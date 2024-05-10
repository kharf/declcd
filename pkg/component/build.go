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

package component

import (
	"errors"
	"fmt"

	internalCue "github.com/kharf/declcd/internal/cue"
	"github.com/kharf/declcd/pkg/helm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrMissingField = errors.New("Missing content field")
)

// Builder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type Builder struct {
}

// NewBuilder contructs a [Builder].
func NewBuilder() Builder {
	return Builder{}
}

// BuildOptions defining which package is compiled and how it is done.
type BuildOptions struct {
	packagePath string
	projectRoot string
}

type buildOptions = func(opts *BuildOptions)

// WithPackagePath provides package path configuration.
func WithPackagePath(packagePath string) buildOptions {
	return func(opts *BuildOptions) {
		opts.packagePath = packagePath
	}
}

// WithProjectRoot provides the path to the project root.
func WithProjectRoot(projectRootPath string) buildOptions {
	return func(opts *BuildOptions) {
		opts.projectRoot = projectRootPath
	}
}

const (
	ProjectRootPath = "."
)

// Build accepts options defining which cue package to compile
// and compiles it to a slice of component Instances.
func (b Builder) Build(opts ...buildOptions) ([]Instance, error) {
	options := &BuildOptions{
		packagePath: "",
		projectRoot: ProjectRootPath,
	}
	for _, opt := range opts {
		opt(options)
	}
	value, err := internalCue.BuildPackage(
		options.packagePath,
		options.projectRoot,
	)
	if err != nil {
		return nil, err
	}
	iter, err := value.Fields()
	if err != nil {
		return nil, err
	}
	instances := make([]Instance, 0)
	for iter.Next() {
		componentValue := iter.Value()
		var instance internalInstance
		if err = componentValue.Decode(&instance); err != nil {
			return nil, err
		}
		switch instance.Type {
		case "Manifest":
			if err := validateManifest(instance); err != nil {
				return nil, err
			}
			instances = append(instances, &Manifest{
				ID:           instance.ID,
				Dependencies: instance.Dependencies,
				Content: unstructured.Unstructured{
					Object: instance.Content,
				},
			})
		case "HelmRelease":
			instances = append(instances, &HelmRelease{
				ID:           instance.ID,
				Dependencies: instance.Dependencies,
				Content: helm.ReleaseDeclaration{
					Name:      instance.Name,
					Namespace: instance.Namespace,
					Chart:     instance.Chart,
					Values:    instance.Values,
				},
			})
		}
	}
	return instances, nil
}

func validateManifest(instance internalInstance) error {
	_, found := instance.Content["apiVersion"]
	if !found {
		return missingFieldError("apiVersion")
	}
	_, found = instance.Content["kind"]
	if !found {
		return missingFieldError("kind")
	}
	metadata, ok := instance.Content["metadata"].(map[string]interface{})
	if !ok {
		return fmt.Errorf(
			"%w: %s field not found or wrong format",
			ErrMissingField,
			"metadata",
		)
	}
	_, found = metadata["name"]
	if !found {
		return missingFieldError("metadata.name")
	}
	_, found = metadata["namespace"]
	if !found {
		return missingFieldError("metadata.namespace")
	}
	return nil
}

func missingFieldError(field string) error {
	return fmt.Errorf("%w: %s field not found", ErrMissingField, field)
}
