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
	"github.com/kharf/navecd/pkg/helm"
	"github.com/kharf/navecd/pkg/kube"
)

// Instance represents a Navecd component with its id, dependencies and content.
// It is the Go equivalent of the CUE definition the user interacts with.
// ID is constructed based on the content of the component.
type Instance interface {
	GetID() string
	GetDependencies() []string
}

var _ Instance = (*kube.Manifest)(nil)
var _ Instance = (*helm.ReleaseComponent)(nil)
