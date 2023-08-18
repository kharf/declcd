package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
)

var (
	ErrWrongComponentFormat = errors.New("wrong component format")
)

// ComponentBuilder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type ComponentBuilder struct {
	ctx *cue.Context
}

// NewComponentBuilder contructs a [ComponentBuilder] with given CUE context.
func NewComponentBuilder(ctx *cue.Context) ComponentBuilder {
	return ComponentBuilder{
		ctx: ctx,
	}
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
	ctx := b.ctx
	options := &ComponentBuildOptions{
		componentPath: "",
		projectRoot:   ProjectRootPath,
	}

	for _, opt := range opts {
		opt(options)
	}

	cfg := &load.Config{
		Package:    filepath.Base(options.componentPath),
		ModuleRoot: options.projectRoot,
		Dir:        options.projectRoot,
	}

	packagePath := options.componentPath
	harmonizedPackagePath := packagePath
	currentDirectoryPrefix := "./"
	if !strings.HasPrefix(packagePath, currentDirectoryPrefix) {
		harmonizedPackagePath = currentDirectoryPrefix + packagePath
	}

	instances := load.Instances([]string{harmonizedPackagePath}, cfg)
	if len(instances) > 1 {
		return nil, fmt.Errorf("%w: too many cue instances found. Make sure to only use a single cue package for your component.", ErrWrongComponentFormat)
	}

	instance := instances[0]
	if instance.Err != nil {
		return nil, instance.Err
	}

	value := ctx.BuildInstance(instance)
	if value.Err() != nil {
		return nil, value.Err()
	}

	err := value.Validate()
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

	return &component, nil
}
