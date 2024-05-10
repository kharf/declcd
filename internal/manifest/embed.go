package manifest

import _ "embed"

//go:embed system.cue
var System string

//go:embed crd.cue
var CRD string

//go:embed project.cue
var Project string
