package api

// Defines the CUE schema of decl's entry manifests.
const EntrySchema = `
#entry: [Name=_]: {
	name:             Name
	intervalSeconds: uint | *60
	dependencies?: [...string]
}
entry: #entry
`

// EntryDef is an identifiable entry definition, corresponding to the schema. It always only contains one key, which is the name of the [Entry].
type EntryDef struct {
	EntriesByName map[string]Entry `json:"entry"`
}

// Entry is the entrypoint of a decl package. It used to define the package's dependencies and its reconciliation interval.
type Entry struct {
	Name            string   `json: "name"`
	IntervalSeconds int      `json: "intervalSeconds"`
	Dependencies    []string `json: "dependencies"`
}
