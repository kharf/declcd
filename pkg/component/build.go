package component

import (
	"errors"

	"github.com/kharf/declcd/internal/cue"
)

var (
	ErrWrongComponentFormat = errors.New("wrong component format")
)

// Builder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type Builder struct {
}

// NewBuilder contructs a [Builder] with given CUE context.
func NewBuilder() Builder {
	return Builder{}
}

// BuildOptions defining what component is compiled and how it is done.
type BuildOptions struct {
	componentPath string
	projectRoot   string
}

type buildOptions = func(opts *BuildOptions)

// WithComponentPath provides component path configuration.
func WithComponentPath(componentPath string) buildOptions {
	return func(opts *BuildOptions) {
		opts.componentPath = componentPath
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

// Build accepts options defining which component to compile and how it is done and compiles it to a k8s unstructured API object/struct.
func (b Builder) Build(opts ...buildOptions) (*Instance, error) {
	options := &BuildOptions{
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
	var component Instance
	err = componentValue.Decode(&component)
	if err != nil {
		return nil, err
	}
	return &component, nil
}
