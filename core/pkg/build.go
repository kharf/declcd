package core

import (
	"errors"
	"io/fs"

	"cuelang.org/go/cue"
	"github.com/kharf/declcd/core/api"
)

var (
	ErrWrongEntryFormat = errors.New("wrong entry format")
)

// EntryBuilder compiles and decodes CUE entry definitions to the corresponding Go struct.
type EntryBuilder interface {
	Build(entry string) (*api.Entry, error)
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
func (b ContentEntryBuilder) Build(entryContent string) (*api.Entry, error) {
	ctx := b.ctx

	specVal := ctx.CompileString(api.EntrySchema)
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

	var entryDef api.EntryDef
	err = unifiedVal.Decode(&entryDef)
	if err != nil {
		return nil, err
	}

	if len(entryDef.EntriesByName) != 1 {
		return nil, ErrWrongEntryFormat
	}

	var entry api.Entry
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
func (b FileEntryBuilder) Build(entryFilePath string) (*api.Entry, error) {
	entryContent, err := fs.ReadFile(b.fs, entryFilePath)
	if err != nil {
		return nil, err
	}
	return b.entryBuilder.Build(string(entryContent))
}
