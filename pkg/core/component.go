package core

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Defines the CUE schema of decl's components.
const ComponentSchema = `
#component: {
	intervalSeconds: uint | *60
	manifests: [...]
}
`

const (
	ComponentFileName = "component.cue"
)

// Component defines the component's manifests and its reconciliation interval.
type Component struct {
	IntervalSeconds int                         `json:"intervalSeconds"`
	Manifests       []unstructured.Unstructured `json:"manifests"`
}
