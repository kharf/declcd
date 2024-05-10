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

package component

import (
	"github.com/kharf/declcd/pkg/helm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Instance represents a Declcd component with its id, dependencies and content.
// It is the Go equivalent of the CUE definition the user interacts with.
// ID is constructed based on the content of the component.
type Instance interface {
	GetID() string
	GetDependencies() []string
}

// internalInstance represents a Declcd component with its id, dependencies and content.
// It is the Go equivalent of the Component CUE definition the user interacts with.
type internalInstance struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Dependencies []string               `json:"dependencies"`
	Content      map[string]interface{} `json:"content"`
	Name         string                 `json:"name"`
	Namespace    string                 `json:"namespace"`
	Chart        helm.Chart             `json:"chart"`
	Values       map[string]interface{} `json:"values"`
}

// Manifest represents a Declcd component with its id, dependencies and content.
// It is the Go equivalent of the CUE definition the user interacts with.
// See [unstructured.Unstructured] for more.
type Manifest struct {
	ID           string
	Dependencies []string
	Content      unstructured.Unstructured
}

var _ Instance = (*Manifest)(nil)

func (m *Manifest) GetID() string {
	return m.ID
}

func (m *Manifest) GetDependencies() []string {
	return m.Dependencies
}

// HelmRelease represents a Declcd component with its id, dependencies and content..
// It is the Go equivalent of the CUE definition the user interacts with.
// See [helm.ReleaseDeclaration] for more.
type HelmRelease struct {
	ID           string
	Dependencies []string
	Content      helm.ReleaseDeclaration
}

var _ Instance = (*HelmRelease)(nil)

func (hr *HelmRelease) GetID() string {
	return hr.ID
}
func (hr *HelmRelease) GetDependencies() []string {
	return hr.Dependencies
}
