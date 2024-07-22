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

package helm

import (
	"strings"

	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReleaseComponent represents a Declcd component with its id, dependencies and content.
// It is the Go equivalent of the CUE definition the user interacts with.
// See [ReleaseDeclaration] for more.
type ReleaseComponent struct {
	ID           string
	Dependencies []string
	Content      ReleaseDeclaration
}

func (hr *ReleaseComponent) GetID() string {
	return hr.ID
}
func (hr *ReleaseComponent) GetDependencies() []string {
	return hr.Dependencies
}

type Release = ReleaseDeclaration

// ReleaseDeclaration is a Declaration of the desired state (Release) in a Git repository.
// A Helm Release is a running instance of a Chart and the current state in a Kubernetes Cluster.
type ReleaseDeclaration struct {
	// Name influences the name of the installed objects of a Helm Chart.
	// When set, the installed objects are suffixed with the chart name.
	// Defaults to the chart name.
	Name string `json:"name"`

	// Namespace specifies the Kubernetes namespace to which the Helm Chart is installed to.
	// Defaults to default.
	Namespace string `json:"namespace"`

	// A Helm package that contains information
	// sufficient for installing a set of Kubernetes resources into a Kubernetes cluster.
	Chart Chart `json:"chart"`

	// Values provide a way to override Helm Chart template defaults with custom information.
	Values Values `json:"values"`

	// Patches allow to overwrite rendered manifests before installing/upgrading.
	// Additionally they can be used to attach build attributes to fields.
	Patches *Patches `json:"patches"`

	// Helm CRD handling configuration.
	CRDs CRDs `json:"crds"`

	// Version is an int which represents the revision of the release.
	// Not declared by users.
	Version int `json:"-"`
}

// Helm CRD handling configuration.
type CRDs struct {
	// Helm only supports installation by default.
	// This option extends Helm to allow Declcd to upgrade CRDs packaged with a Chart.
	AllowUpgrade bool `json:"allowUpgrade"`
}

// Values provide a way to override Helm Chart template defaults with custom information.
type Values map[string]interface{}

// Patches allow to overwrite rendered manifests before installing/upgrading.
// Additionally they can be used to attach build attributes to fields.
type Patches struct {
	Unstructureds map[string]kube.ExtendedUnstructured
}

func NewPatches() *Patches {
	return &Patches{
		Unstructureds: map[string]kube.ExtendedUnstructured{},
	}
}

func (p *Patches) Put(unstructured kube.ExtendedUnstructured) {
	var namespace string
	if unstructured.GetNamespace() == "" {
		namespace = "default"
	} else {
		namespace = unstructured.GetNamespace()
	}

	sb := strings.Builder{}
	sb.WriteString(unstructured.GetAPIVersion())
	sb.WriteString("-")
	sb.WriteString(unstructured.GetKind())
	sb.WriteString("-")
	sb.WriteString(namespace)
	sb.WriteString("-")
	sb.WriteString(unstructured.GetName())

	p.Unstructureds[sb.String()] = unstructured
}

func (p *Patches) Get(
	name string,
	namespace string,
	typeMeta v1.TypeMeta,
) *kube.ExtendedUnstructured {
	if namespace == "" {
		namespace = "default"
	}

	sb := strings.Builder{}
	sb.WriteString(typeMeta.APIVersion)
	sb.WriteString("-")
	sb.WriteString(typeMeta.Kind)
	sb.WriteString("-")
	sb.WriteString(namespace)
	sb.WriteString("-")
	sb.WriteString(name)

	unstr, found := p.Unstructureds[sb.String()]
	if !found {
		return nil
	}

	return &unstr
}
