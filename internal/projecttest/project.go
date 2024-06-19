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

package projecttest

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"github.com/otiai10/copy"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Project struct {
	Name          string
	GitRepository *gittest.LocalGitRepository
	TargetPath    string
}

type Environment struct {
	Log            logr.Logger
	ProjectManager project.Manager
	TestRoot       string
	Projects       []Project
	*kubetest.Environment
}

func (env *Environment) Stop() {
	if env.Environment != nil {
		env.Environment.Stop()
	}
	_ = os.RemoveAll(env.TestRoot)

	for _, project := range env.Projects {
		_ = os.RemoveAll(project.GitRepository.Directory)
	}
}

type Option interface {
	Apply(opts *options)
}

type options struct {
	projectSources []string
	kubeOpts       []kubetest.Option
}

type WithProjectSource string

var _ Option = (*WithProjectSource)(nil)

func (opt WithProjectSource) Apply(opts *options) {
	opts.projectSources = append(opts.projectSources, string(opt))
}

type withKubernetes []kubetest.Option

func WithKubernetes(opts ...kubetest.Option) withKubernetes {
	return opts
}

var _ Option = (*WithProjectSource)(nil)

func (opt withKubernetes) Apply(opts *options) {
	opts.kubeOpts = opt
}

func StartProjectEnv(t testing.TB, opts ...Option) Environment {
	options := options{}
	for _, o := range opts {
		o.Apply(&options)
	}
	if len(options.projectSources) == 0 {
		options.projectSources = []string{"simple"}
	}

	logOpts := ctrlZap.Options{
		Development: false,
		Level:       zapcore.Level(-1),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))

	testRoot, err := os.MkdirTemp("", "declcd-*")
	assert.NilError(t, err)

	var projects []Project
	for _, source := range options.projectSources {
		testProject, err := os.MkdirTemp(testRoot, fmt.Sprintf("%s-*", source))
		assert.NilError(t, err)

		err = copy.Copy(filepath.Join("test/testdata", source), testProject)
		assert.NilError(t, err)

		repo, err := gittest.InitGitRepository(testProject)
		assert.NilError(t, err)
		projects = append(projects, Project{
			Name:          source,
			GitRepository: repo,
			TargetPath:    testProject,
		})
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	// set to to true globally as CUE for example uses the DefaultTransport
	http.DefaultTransport = transport

	env := kubetest.StartKubetestEnv(t, log, options.kubeOpts...)
	projectManager := project.NewManager(component.NewBuilder(), log, runtime.GOMAXPROCS(0))

	return Environment{
		ProjectManager: projectManager,
		TestRoot:       testRoot,
		Projects:       projects,
		Environment:    env,
		Log:            log,
	}
}
