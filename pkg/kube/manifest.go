// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ManifestIgnoreAttribute is a CUE build attribute a user can define on a field or declaration
// to tell Declcd to ignore fields or structs when applying Kubernetes Manifests.
type ManifestIgnoreAttribute int

const (
	// Default. Declcd will enforce the field/struct.
	None ManifestIgnoreAttribute = iota

	// This tells Declcd to omit the field/struct 'tagged' with this value on a retry ssa patch request.
	OnConflict
)

// ManifestMetadata extends unstructured fields with additional information.
type ManifestMetadata interface {
	Metadata() *ManifestFieldMetadata
}

type ManifestMetadataNode map[string]ManifestMetadata

var _ ManifestMetadata = (*ManifestMetadataNode)(nil)

func (s *ManifestMetadataNode) Metadata() *ManifestFieldMetadata {
	return nil
}

type ManifestFieldMetadata struct {
	IgnoreAttr ManifestIgnoreAttribute
}

var _ ManifestMetadata = (*ManifestFieldMetadata)(nil)

func (v *ManifestFieldMetadata) Metadata() *ManifestFieldMetadata {
	return v
}

type ManifestAttributeInfo struct {
	HasIgnoreConflictAttributes bool
}

// ExtendedUnstructured enhances Kubernetes Unstructured struct with additional Metadata, like IgnoreAttributes.
type ExtendedUnstructured struct {
	*unstructured.Unstructured
	Metadata      ManifestMetadata      `json:"-"`
	AttributeInfo ManifestAttributeInfo `json:"-"`
}

// Manifest represents a Declcd component with its id, dependencies and content.
// It is the Go equivalent of the CUE definition the user interacts with.
// See [unstructured.Unstructured] for more.
type Manifest struct {
	ID           string
	Dependencies []string
	Content      ExtendedUnstructured
}

func (m *Manifest) GetID() string {
	return m.ID
}

func (m *Manifest) GetDependencies() []string {
	return m.Dependencies
}

func (m *Manifest) GetKind() string {
	return m.Content.GetKind()
}

func (m *Manifest) GetAPIVersion() string {
	return m.Content.GetAPIVersion()
}

func (m *Manifest) GetLabels() map[string]string {
	return m.Content.GetLabels()
}

func (m *Manifest) GetName() string {
	return m.Content.GetName()
}

func (m *Manifest) GetNamespace() string {
	return m.Content.GetNamespace()
}
