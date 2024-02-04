package component

import (
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	FileName = "component.cue"
)

// Instance represents a Declcd component with its id, dependencies, rendered manifests and helm releases.
// It is the Go equivalent of the CUE definition the user interacts with.
type Instance struct {
	ID           string                      `json:"id"`
	Dependencies []string                    `json:"dependencies"`
	Manifests    []unstructured.Unstructured `json:"manifests"`
	HelmReleases []helm.ReleaseDeclaration   `json:"helmReleases"`
}

func New(id string, dependencies []string, manifests []unstructured.Unstructured, helmReleases []helm.ReleaseDeclaration) Instance {
	return Instance{
		ID:           id,
		Dependencies: dependencies,
		Manifests:    manifests,
		HelmReleases: helmReleases,
	}
}

// Node represents a Declcd component with its id, dependencies and manifest metadata inside a directed acyclic graph.
// This object is a smaller representation of [Instance] with only references and metadata.
type Node struct {
	id           string
	path         string
	dependencies []string
	manifests    []kube.ManifestMetadata
	helmReleases []helm.ReleaseMetadata
}

func NewNode(id string, path string, dependencies []string, manifests []kube.ManifestMetadata, helmReleases []helm.ReleaseMetadata) Node {
	return Node{
		id:           id,
		path:         path,
		dependencies: dependencies,
		manifests:    manifests,
		helmReleases: helmReleases,
	}
}

func (node Node) ID() string {
	return node.id
}

func (node Node) Path() string {
	return node.path
}

func (node Node) Dependencies() []string {
	return node.dependencies
}

func (node Node) Manifests() []kube.ManifestMetadata {
	return node.manifests
}

func (node Node) HelmReleases() []helm.ReleaseMetadata {
	return node.helmReleases
}
