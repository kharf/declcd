package core

import (
	"errors"

	"cuelang.org/go/cue"
	"github.com/kharf/declcd/core/api"
)

var (
	ErrWrongEntryFormat = errors.New("wrong entry format")
)

// EntryBuilder compiles and decodes CUE entry definitions to the corresponding Go struct.
type EntryBuilder struct {
	ctx *cue.Context
}

// NewEntryBuilder contructs an [EntryBuilder] with given CUE context
func NewEntryBuilder(ctx *cue.Context) EntryBuilder {
	return EntryBuilder{
		ctx: ctx,
	}
}

// Build accepts an entry CUE definition as string and compiles it to the corresponding Go struct.
func (b EntryBuilder) Build(plainEntry string) (*api.Entry, error) {
	ctx := b.ctx

	specVal := ctx.CompileString(api.EntrySchema)
	if specVal.Err() != nil {
		return nil, specVal.Err()
	}

	val := ctx.CompileString(plainEntry)
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
