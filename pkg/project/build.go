package project

import (
	"errors"

	"github.com/kharf/declcd/internal/cue"
)

var (
	ErrWrongComponentFormat = errors.New("wrong component format")
)

// ComponentBuilder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type ComponentBuilder struct {
}

// NewComponentBuilder contructs a [ComponentBuilder] with given CUE context.
func NewComponentBuilder() ComponentBuilder {
	return ComponentBuilder{}
}

// ComponentBuildOptions defining what component is compiled and how it is done.
type ComponentBuildOptions struct {
	componentPath string
	projectRoot   string
}

type componentBuildOptions = func(opts *ComponentBuildOptions)

// WithComponentPath provides component path configuration.
func WithComponentPath(componentPath string) componentBuildOptions {
	return func(opts *ComponentBuildOptions) {
		opts.componentPath = componentPath
	}
}

// WithProjectRoot provides the path to the project root.
func WithProjectRoot(projectRootPath string) componentBuildOptions {
	return func(opts *ComponentBuildOptions) {
		opts.projectRoot = projectRootPath
	}
}

const (
	ProjectRootPath = "."
)

// Build accepts options defining which component is to be compiled and how it is done and compiles it to a k8s unstructured API object/struct.
func (b ComponentBuilder) Build(opts ...componentBuildOptions) (*Component, error) {
	options := &ComponentBuildOptions{
		componentPath: "",
		projectRoot:   ProjectRootPath,
	}
	for _, opt := range opts {
		opt(options)
	}
	value, err := cue.BuildPackage(options.componentPath, options.projectRoot)
	if err != nil {
		return nil, err
	}
	iter, err := value.Fields()
	if err != nil {
		return nil, err
	}
	iter.Next()
	componentValue := iter.Value()
	var component Component
	err = componentValue.Decode(&component)
	if err != nil {
		return nil, err
	}
	component.cueValue = value
	return &component, nil
}
