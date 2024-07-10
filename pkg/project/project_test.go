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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func setUp() logr.Logger {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logOpts := ctrlZap.Options{
		DestWriter:  io.Discard,
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	return log
}

func TestManager_Load(t *testing.T) {
	var err error
	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	registryPath, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assertError(err)
	defer cueModuleRegistry.Close()

	env := projecttest.StartProjectEnv(t,
		projecttest.WithProjectSource("simple"),
		projecttest.WithKubernetes(
			kubetest.WithEnabled(false),
		),
	)
	defer env.Stop()
	testProject := env.Projects[0]
	err = helmtest.ReplaceTemplate(
		helmtest.Template{
			Name:                    "test",
			TestProjectPath:         testProject.TargetPath,
			RelativeReleaseFilePath: "infra/prometheus/releases.cue",
			RepoURL:                 "oci://empty",
		},
		testProject.GitRepository,
	)
	assert.NilError(t, err)

	logger := setUp()
	root := testProject.TargetPath

	pm := project.NewManager(component.NewBuilder(), logger, runtime.GOMAXPROCS(0))
	dag, err := pm.Load(root)
	assert.NilError(t, err)

	linkerd := dag.Get("linkerd___Namespace")
	assert.Assert(t, linkerd != nil)
	linkerdManifest, ok := linkerd.(*component.Manifest)
	assert.Assert(t, ok)
	assert.Assert(t, linkerdManifest.GetAPIVersion() == "v1")
	assert.Assert(t, linkerdManifest.GetKind() == "Namespace")
	assert.Assert(t, linkerdManifest.GetName() == "linkerd")

	prometheus := dag.Get("prometheus___Namespace")
	assert.Assert(t, prometheus != nil)
	prometheusRelease := dag.Get("test_prometheus_HelmRelease")
	assert.Assert(t, prometheusRelease != nil)
	subcomponent := dag.Get("mysubcomponent_prometheus_apps_Deployment")
	assert.Assert(t, subcomponent != nil)
}

var dagResult *component.DependencyGraph

func BenchmarkManager_Load(b *testing.B) {
	b.ReportAllocs()
	logger := setUp()
	cwd, err := os.Getwd()
	assert.NilError(b, err)
	root := filepath.Join(cwd, "test", "testdata", "complex")
	pm := project.NewManager(component.NewBuilder(), logger, runtime.GOMAXPROCS(0))
	b.ResetTimer()
	var dag *component.DependencyGraph
	for n := 0; n < b.N; n++ {
		dag, err = pm.Load(root)
	}
	dagResult = dag
}
