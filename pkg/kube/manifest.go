// Copyright 2024 Google LLC
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
