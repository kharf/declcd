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
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	gitops "github.com/kharf/declcd/api/v1beta1"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/manifest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	intervalInSeconds = 5
	url               = "git@github.com:kharf/declcd.git"
	branch            = "main"
)

type testProject struct {
	name        string
	shard       string
	isSecondary bool
}

func TestInstallAction_Install(t *testing.T) {
	testRoot, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	cueModuleRegistry, err := ocitest.StartCUERegistry(testRoot)
	assertError(err)
	defer cueModuleRegistry.Close()

	testCases := []struct {
		name      string
		project   testProject
		assertion func(env projecttest.Environment, testProject testProject)
		post      func(env projecttest.Environment, action project.InstallAction, testProjectCR testProject)
	}{
		{
			name: "Fresh",
			project: testProject{
				name:        "fresh",
				shard:       "fresh",
				isSecondary: false,
			},
			assertion: func(env projecttest.Environment, testProject testProject) {
				defaultAssertion(t, env, testProject)
			},
			post: func(env projecttest.Environment, action project.InstallAction, testProject testProject) {},
		},
		{
			name: "MultiTenancy",
			project: testProject{
				name:        "primary",
				shard:       "primary",
				isSecondary: false,
			},
			assertion: func(env projecttest.Environment, testProject testProject) {
				defaultAssertion(t, env, testProject)
			},
			post: func(env projecttest.Environment, action project.InstallAction, testProjectCR testProject) {
				err = project.Init(
					"github.com/kharf/declcd/installation",
					"secondary",
					true,
					env.Projects[0].TargetPath,
					"1.0.0",
				)
				assert.NilError(t, err)

				server, client := gittest.MockGitProvider(
					t,
					vcs.GitHub,
					fmt.Sprintf("declcd-%s", "secondary"),
				)
				defer server.Close()

				kubeClient, err := kube.NewDynamicClient(env.ControlPlane.Config)
				assert.NilError(t, err)

				action = project.NewInstallAction(kubeClient, client, env.Projects[0].TargetPath)
				ctx := context.Background()
				err = action.Install(
					ctx,
					project.InstallOptions{
						Name:     "secondary",
						Shard:    "secondary",
						Branch:   branch,
						Interval: intervalInSeconds,
						Url:      url,
						Token:    "aaaa",
					},
				)
				assert.NilError(t, err)

				defaultAssertion(t, env, testProjectCR)
				defaultAssertion(t, env, testProject{
					name:        "secondary",
					shard:       "secondary",
					isSecondary: true,
				},
				)
			},
		},
		{
			name: "RunTwice",
			project: testProject{
				name:        "runtwice",
				shard:       "runtwice",
				isSecondary: false,
			},
			assertion: func(env projecttest.Environment, testProject testProject) {
				defaultAssertion(t, env, testProject)
			},
			post: func(env projecttest.Environment, action project.InstallAction, testProject testProject) {
				ctx := context.Background()
				var getSecret func() v1.Secret
				getSecret = func() v1.Secret {
					var vcsKey v1.Secret
					err := env.TestKubeClient.Get(
						ctx,
						types.NamespacedName{
							Name:      vcs.SecretName(testProject.name),
							Namespace: project.ControllerNamespace,
						},
						&vcsKey,
					)
					assert.NilError(t, err)
					return vcsKey
				}
				vcsKeyBefore := getSecret()
				err := action.Install(
					ctx,
					project.InstallOptions{
						Branch:   branch,
						Interval: intervalInSeconds,
						Name:     testProject.name,
						Shard:    testProject.shard,
						Url:      url,
						Token:    "aaaa",
					},
				)
				assert.NilError(t, err)
				defaultAssertion(t, env, testProject)
				vcsKeyAfter := getSecret()
				assert.Assert(t, cmp.Equal(vcsKeyAfter.Data, vcsKeyBefore.Data))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, client := gittest.MockGitProvider(
				t,
				vcs.GitHub,
				fmt.Sprintf("declcd-%s", tc.project.name),
			)
			defer server.Close()

			env := projecttest.StartProjectEnv(t)
			defer env.Stop()

			ctx := context.Background()
			kubeClient, err := kube.NewDynamicClient(env.ControlPlane.Config)
			assert.NilError(t, err)

			testProject := env.Projects[0]
			err = project.Init(
				"github.com/kharf/declcd/installation",
				tc.project.shard,
				tc.project.isSecondary,
				testProject.TargetPath,
				"1.0.0",
			)
			assert.NilError(t, err)

			action := project.NewInstallAction(kubeClient, client, testProject.TargetPath)

			err = action.Install(
				ctx,
				project.InstallOptions{
					Name:     tc.project.name,
					Shard:    tc.project.shard,
					Branch:   branch,
					Interval: intervalInSeconds,
					Url:      url,
					Token:    "aaaa",
				},
			)
			assert.NilError(t, err)

			tc.assertion(env, tc.project)

			tc.post(env, action, tc.project)
		})
	}
}

func defaultAssertion(
	t *testing.T,
	env projecttest.Environment,
	testProjectCR testProject,
) {
	ctx := context.Background()
	var ns v1.Namespace
	err := env.TestKubeClient.Get(ctx, types.NamespacedName{Name: project.ControllerNamespace}, &ns)
	assert.NilError(t, err)

	projectName := testProjectCR.name

	var deployment appsv1.Deployment
	controllerName := fmt.Sprintf("%s-%s", "project-controller", testProjectCR.shard)
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{
			Name:      controllerName,
			Namespace: project.ControllerNamespace,
		},
		&deployment,
	)
	assert.NilError(t, err)

	var gitOpsProject gitops.GitOpsProject
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: projectName, Namespace: project.ControllerNamespace},
		&gitOpsProject,
	)
	assert.NilError(t, err)

	testProject := env.Projects[0]
	projectFile, err := os.Open(
		filepath.Join(
			testProject.TargetPath,
			fmt.Sprintf("declcd/%s_project.cue", testProjectCR.name),
		),
	)
	assert.NilError(t, err)

	projectContent, err := io.ReadAll(projectFile)
	assert.NilError(t, err)

	var projectBuf bytes.Buffer
	projectTmpl, err := template.New("").Parse(manifest.Project)

	assert.NilError(t, err)
	err = projectTmpl.Execute(&projectBuf, map[string]interface{}{
		"Name":                projectName,
		"Namespace":           project.ControllerNamespace,
		"Branch":              branch,
		"PullIntervalSeconds": intervalInSeconds,
		"Url":                 url,
		"Shard":               testProjectCR.shard,
	})
	assert.NilError(t, err)

	assert.Equal(t, string(projectContent), projectBuf.String())

	var vcsKey v1.Secret
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{
			Name:      vcs.SecretName(projectName),
			Namespace: project.ControllerNamespace,
		},
		&vcsKey,
	)
	assert.NilError(t, err)

	var knownHostsCm v1.ConfigMap
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: "known-hosts", Namespace: project.ControllerNamespace},
		&knownHostsCm,
	)
	assert.NilError(t, err)

	var service v1.Service
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: controllerName, Namespace: project.ControllerNamespace},
		&service,
	)
	assert.NilError(t, err)
}
