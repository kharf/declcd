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
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/txtar"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Environment struct {
	Log              logr.Logger
	TestRoot         string
	TestProject      string
	LocalTestProject string
	GitRepository    *gittest.LocalGitRepository
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

func InitTestEnvironment(t testing.TB, txtarData []byte) Environment {
	testRoot := t.TempDir()

	localProject, err := os.MkdirTemp(testRoot, "")
	assert.NilError(t, err)

	err = txtar.Create(localProject, bytes.NewReader(txtarData))
	assert.NilError(t, err)

	remoteProject, err := os.MkdirTemp(testRoot, "")
	assert.NilError(t, err)

	gitRepository, err := gittest.InitGitRepository(t, remoteProject, localProject, "main")
	assert.NilError(t, err)

	logOpts := ctrlZap.Options{
		Development: false,
		Level:       zapcore.Level(-1),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	return Environment{
		TestRoot:         testRoot,
		TestProject:      remoteProject,
		LocalTestProject: localProject,
		GitRepository:    gitRepository,
		Log:              log,
	}
}

type Template struct {
	TestProjectPath  string
	RelativeFilePath string
	Data             any
}

func ReplaceTemplate(
	tmpl Template,
	gitRepository *gittest.LocalGitRepository,
) error {
	releasesFilePath := filepath.Join(
		tmpl.TestProjectPath,
		tmpl.RelativeFilePath,
	)

	releasesContent, err := os.ReadFile(releasesFilePath)
	if err != nil {
		return err
	}

	parsedTemplate, err := template.New("").Parse(string(releasesContent))
	if err != nil {
		return err
	}

	releasesFile, err := os.Create(releasesFilePath)
	if err != nil {
		return err
	}
	defer releasesFile.Close()

	err = parsedTemplate.Execute(releasesFile, tmpl.Data)
	if err != nil {
		return err
	}

	_, err = gitRepository.CommitFile(
		tmpl.RelativeFilePath,
		"overwrite template",
	)
	if err != nil {
		return err
	}

	return nil
}
