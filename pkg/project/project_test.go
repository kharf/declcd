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

package project_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kharf/navecd/internal/dnstest"
	"github.com/kharf/navecd/internal/ocitest"
	"github.com/kharf/navecd/internal/projecttest"
	"github.com/kharf/navecd/internal/testtemplates"
	"github.com/kharf/navecd/pkg/component"
	"github.com/kharf/navecd/pkg/project"
	"gotest.tools/v3/assert"
)

func useManagerTemplate() string {
	return fmt.Sprintf(`
-- cue.mod/module.cue --
module: "github.com/kharf/navecd/internal/controller/projectone@v0"
language: version: "%s"
deps: {
	"github.com/kharf/navecd/schema@v0": {
		v: "v0.0.99"
	}
}

-- infra/toola/namespace.cue --
package toola

import (
	"github.com/kharf/navecd/schema/component"
)

#namespace: {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: "toola"
}

ns: component.#Manifest & {
	content: #namespace
}

-- infra/toolb/namespace.cue --
package toolb

import (
	"github.com/kharf/navecd/schema/component"
)

#namespace: {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: "toolb"
}

ns: component.#Manifest & {
	content: #namespace
}
`, testtemplates.ModuleVersion)
}

func TestManager_Load(t *testing.T) {
	var err error
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	cueModuleRegistry, err := ocitest.StartCUERegistry(t.TempDir())
	assert.NilError(t, err)
	defer cueModuleRegistry.Close()

	env := projecttest.InitTestEnvironment(t, []byte(useManagerTemplate()))

	pm := project.NewManager(component.NewBuilder(), runtime.GOMAXPROCS(0))
	instance, err := pm.Load(env.LocalTestProject)
	assert.NilError(t, err)

	dag := instance.Dag

	ns := dag.Get("toola___Namespace")
	assert.Assert(t, ns != nil)
	nsManifest, ok := ns.(*component.Manifest)
	assert.Assert(t, ok)
	assert.Assert(t, nsManifest.GetAPIVersion() == "v1")
	assert.Assert(t, nsManifest.GetKind() == "Namespace")
	assert.Assert(t, nsManifest.GetName() == "toola")

	ns = dag.Get("toolb___Namespace")
	assert.Assert(t, ns != nil)
	nsManifest, ok = ns.(*component.Manifest)
	assert.Assert(t, ok)
	assert.Assert(t, nsManifest.GetAPIVersion() == "v1")
	assert.Assert(t, nsManifest.GetKind() == "Namespace")
	assert.Assert(t, nsManifest.GetName() == "toolb")
}

var instance *project.Instance

func BenchmarkManager_Load(b *testing.B) {
	b.ReportAllocs()
	cwd, err := os.Getwd()
	assert.NilError(b, err)
	root := filepath.Join(cwd, "test", "testdata", "complex")
	pm := project.NewManager(component.NewBuilder(), runtime.GOMAXPROCS(0))
	b.ResetTimer()
	var inst *project.Instance
	for n := 0; n < b.N; n++ {
		inst, err = pm.Load(root)
	}
	instance = inst
}
