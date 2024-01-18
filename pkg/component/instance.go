package component

import (
	"fmt"
	"strings"

	"github.com/kharf/declcd/pkg/helm"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	HelmReleases []helm.Release              `json:"helmReleases"`
}

func New(id string, dependencies []string, manifests []unstructured.Unstructured, helmReleases []helm.Release) Instance {
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
	manifests    []ManifestMetadata
	helmReleases []HelmReleaseMetadata
}

func NewNode(id string, path string, dependencies []string, manifests []ManifestMetadata, helmReleases []HelmReleaseMetadata) Node {
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

func (node Node) Manifests() []ManifestMetadata {
	return node.manifests
}

func (node Node) HelmReleases() []HelmReleaseMetadata {
	return node.helmReleases
}

type ManifestMetadata struct {
	v1.TypeMeta
	componentID string
	name        string
	namespace   string
}

func NewManifestMetadata(typeMeta v1.TypeMeta, componentID string, name string, namespace string) ManifestMetadata {
	return ManifestMetadata{
		TypeMeta:    typeMeta,
		componentID: componentID,
		name:        name,
		namespace:   namespace,
	}
}

func (manifest ManifestMetadata) Name() string {
	return manifest.name
}

func (manifest ManifestMetadata) Namespace() string {
	return manifest.namespace
}

func (manifest ManifestMetadata) ComponentID() string {
	return manifest.componentID
}

func (manifest ManifestMetadata) AsKey() string {
	group := ""
	version := ""
	groupVersion := strings.Split(manifest.APIVersion, "/")
	if len(groupVersion) == 1 {
		version = groupVersion[0]
	} else {
		group = groupVersion[0]
		version = groupVersion[1]
	}
	return fmt.Sprintf("%s_%s_%s_%s_%s_%s", manifest.componentID, manifest.name, manifest.namespace, manifest.Kind, group, version)
}

type HelmReleaseMetadata struct {
	componentID string
	name        string
	namespace   string
}

func NewHelmReleaseMetadata(componentID string, name string, namespace string) HelmReleaseMetadata {
	return HelmReleaseMetadata{
		componentID: componentID,
		name:        name,
		namespace:   namespace,
	}
}

func (hr HelmReleaseMetadata) Name() string {
	return hr.name
}

func (hr HelmReleaseMetadata) Namespace() string {
	return hr.namespace
}

func (hr HelmReleaseMetadata) ComponentID() string {
	return hr.componentID
}

func (hr HelmReleaseMetadata) AsKey() string {
	return fmt.Sprintf("%s_%s_%s_%s", hr.componentID, hr.name, hr.namespace, "HelmRelease")
}
