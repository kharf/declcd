package core

import (
	"errors"
	"io/fs"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrWrongEntryFormat    = errors.New("wrong entry format")
	ErrWrongManifestFormat = errors.New("wrong manifest format")
)

// EntryBuilder compiles and decodes CUE entry definitions to the corresponding Go struct.
type EntryBuilder interface {
	Build(entry string) (*Entry, error)
}

// ContentEntryBuilder compiles and decodes CUE entry definitions based on their content to the corresponding Go struct.
type ContentEntryBuilder struct {
	ctx *cue.Context
}

// NewContentEntryBuilder contructs an [EntryBuilder] with given CUE context.
func NewContentEntryBuilder(ctx *cue.Context) ContentEntryBuilder {
	return ContentEntryBuilder{
		ctx: ctx,
	}
}

var _ EntryBuilder = ContentEntryBuilder{}

// Build accepts an entry CUE definition as string and compiles it to the corresponding Go struct.
func (b ContentEntryBuilder) Build(entryContent string) (*Entry, error) {
	ctx := b.ctx

	specVal := ctx.CompileString(EntrySchema)
	if specVal.Err() != nil {
		return nil, specVal.Err()
	}

	val := ctx.CompileString(entryContent)
	if val.Err() != nil {
		return nil, val.Err()
	}

	unifiedVal := specVal.Unify(val)
	if unifiedVal.Err() != nil {
		return nil, unifiedVal.Err()
	}

	err := unifiedVal.Validate()
	if err != nil {
		return nil, err
	}

	var entryDef EntryDef
	err = unifiedVal.Decode(&entryDef)
	if err != nil {
		return nil, err
	}

	if len(entryDef.EntriesByName) != 1 {
		return nil, ErrWrongEntryFormat
	}

	var entry Entry
	for _, e := range entryDef.EntriesByName {
		entry = e
		break
	}

	return &entry, nil
}

// FileEntryBuilder compiles and decodes CUE entry definitions from a FileSystem to the corresponding Go struct.
type FileEntryBuilder struct {
	ctx          *cue.Context
	fs           fs.FS
	entryBuilder ContentEntryBuilder
}

// NewFileEntryBuilder contructs an [EntryBuilder] with given CUE context and FileSystem.
func NewFileEntryBuilder(ctx *cue.Context, fs fs.FS, entryBuilder ContentEntryBuilder) FileEntryBuilder {
	return FileEntryBuilder{
		ctx:          ctx,
		fs:           fs,
		entryBuilder: entryBuilder,
	}
}

var _ EntryBuilder = FileEntryBuilder{}

// Build accepts a path to an entry file and compiles it to the corresponding Go struct.
func (b FileEntryBuilder) Build(entryFilePath string) (*Entry, error) {
	entryContent, err := fs.ReadFile(b.fs, entryFilePath)
	if err != nil {
		return nil, err
	}
	return b.entryBuilder.Build(string(entryContent))
}

// ComponentManifestBuilder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type ComponentManifestBuilder struct {
	ctx *cue.Context
}

// NewComponentManifestBuilder contructs a [ComponentManifestBuilder] with given CUE context.
func NewComponentManifestBuilder(ctx *cue.Context) ComponentManifestBuilder {
	return ComponentManifestBuilder{
		ctx: ctx,
	}
}

// ManifestBuildOptions defining what component is compiled and how it is done.
type ManifestBuildOptions struct {
	componentName string
	componentPath string
	projectRoot   string
}

type manifestBuildOption = func(opts *ManifestBuildOptions)

// WithComponent provides component name and path configuration.
func WithComponent(componentName, componentPath string) manifestBuildOption {
	return func(opts *ManifestBuildOptions) {
		opts.componentName = componentName
		opts.componentPath = componentPath
	}
}

// WithProjectRoot provides the path to the project root.
func WithProjectRoot(projectRootPath string) manifestBuildOption {
	return func(opts *ManifestBuildOptions) {
		opts.projectRoot = projectRootPath
	}
}

const (
	AllPackages     = "*"
	ProjectRootPath = "."
)

// Build accepts options defining what component to be compiled and how it is done and compiles it to a k8s unstructured API object/struct.
func (b ComponentManifestBuilder) Build(opts ...manifestBuildOption) ([]unstructured.Unstructured, error) {
	ctx := cuecontext.New()
	options := &ManifestBuildOptions{
		componentName: AllPackages,
		componentPath: "",
		projectRoot:   ProjectRootPath,
	}

	for _, opt := range opts {
		opt(options)
	}

	cfg := &load.Config{
		Package:    options.componentName,
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
	unstructureds := make([]unstructured.Unstructured, 0, len(instances))
	for _, instance := range instances {
		if instance.Err != nil {
			return []unstructured.Unstructured{}, instance.Err
		}

		value := ctx.BuildInstance(instance)
		if value.Err() != nil {
			return []unstructured.Unstructured{}, value.Err()
		}

		err := value.Validate()
		if err != nil {
			return []unstructured.Unstructured{}, err
		}

		iter, err := value.Fields()
		if err != nil {
			return []unstructured.Unstructured{}, err
		}

		for iter.Next() {
			objValue := iter.Value()
			objIter, err := objValue.Fields()
			if err != nil {
				return []unstructured.Unstructured{}, err
			}

			for objIter.Next() {
				idValue := objIter.Value()
				var obj map[string]interface{}
				err = idValue.Decode(&obj)
				if err != nil {
					return nil, err
				}
				unstructureds = append(unstructureds, unstructured.Unstructured{Object: obj})
			}
		}
	}

	return unstructureds, nil
}
