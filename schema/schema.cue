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

package schema

import "strings"

#Manifest: {
	type:          "Manifest"
	_groupVersion: strings.Split(content.apiVersion, "/")
	_group:        string | *""
	if len(_groupVersion) >= 2 {
		_group: _groupVersion[0]
	}
	id: "\(content.metadata.name)_\(content.metadata.namespace)_\(_group)_\(content.kind)"
	dependencies: [...string]
	content: {
		apiVersion!: string & strings.MinRunes(1)
		kind!:       string & strings.MinRunes(1)
		metadata: {
			namespace: string | *""
			name!:     string & strings.MinRunes(1)
			...
		}
		...
	}
}

#HelmRelease: {
	type: "HelmRelease"
	id:   "\(name)_\(namespace)_\(type)"
	dependencies: [...string]
	name!:      string & strings.MinRunes(1)
	namespace!: string
	chart!:     #HelmChart
	values: {...}
}

#HelmChart: {
	name!:    string & strings.MinRunes(1)
	repoURL!: string & strings.HasPrefix("oci://") | strings.HasPrefix("http://") | strings.HasPrefix("https://")
	version!: string & strings.MinRunes(1)
	auth: {
		secretRef: {
			name!:      string & strings.MinRunes(1)
			namespace!: string & strings.MinRunes(1)
		}
	}
}
