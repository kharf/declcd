package kube

import v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
