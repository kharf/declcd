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

package install_test

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
	"github.com/kharf/declcd/internal/install"
	"github.com/kharf/declcd/internal/manifest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/secret"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	name              = "test"
	namespace         = "default"
	intervalInSeconds = 5
	url               = "git@github.com:kharf/declcd.git"
	branch            = "main"
)

func TestMain(m *testing.M) {
	testRoot, err := os.MkdirTemp("", "declcd-cue-registry*")
	assertError(err)

	dnsServer, err := dnstest.NewDNSServer()
	assertError(err)
	defer dnsServer.Close()

	cueModuleRegistry, err := ocitest.StartCUERegistry(testRoot)
	assertError(err)
	defer cueModuleRegistry.Close()

	os.Exit(m.Run())
}

func assertError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func TestAction_Install(t *testing.T) {
	testCases := []struct {
		name      string
		assertion func(env projecttest.Environment, nsName string)
		post      func(env projecttest.Environment, action install.Action, nsName string)
	}{
		{
			name: "Fresh",
			assertion: func(env projecttest.Environment, nsName string) {
				defaultAssertion(t, env, nsName)
			},
			post: func(env projecttest.Environment, action install.Action, nsName string) {
			},
		},
		{
			name: "Idempotence",
			assertion: func(env projecttest.Environment, nsName string) {
				defaultAssertion(t, env, nsName)
			},
			post: func(env projecttest.Environment, action install.Action, nsName string) {
				ctx := context.Background()
				var getSecrets func() (v1.Secret, v1.Secret)
				getSecrets = func() (v1.Secret, v1.Secret) {
					var decKey v1.Secret
					err := env.TestKubeClient.Get(
						ctx,
						types.NamespacedName{Name: secret.K8sSecretName, Namespace: nsName},
						&decKey,
					)
					assert.NilError(t, err)
					var vcsKey v1.Secret
					err = env.TestKubeClient.Get(
						ctx,
						types.NamespacedName{Name: vcs.K8sSecretName, Namespace: nsName},
						&vcsKey,
					)
					assert.NilError(t, err)
					return decKey, vcsKey
				}
				decKeyBefore, vcsKeyBefore := getSecrets()
				err := action.Install(
					ctx,
					install.Namespace(nsName),
					install.Branch(branch),
					install.Interval(intervalInSeconds),
					install.Name(name),
					install.URL(url),
					install.Token("aaaa"),
				)
				assert.NilError(t, err)
				defaultAssertion(t, env, nsName)
				decKeyAfter, vcsKeyAfter := getSecrets()
				assert.Assert(t, cmp.Equal(decKeyAfter.Data, decKeyBefore.Data))
				assert.Assert(t, cmp.Equal(vcsKeyAfter.Data, vcsKeyBefore.Data))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server, client := gittest.MockGitProvider(t, vcs.GitHub)
			defer server.Close()

			env := projecttest.StartProjectEnv(t)
			defer env.Stop()

			ctx := context.Background()
			kubeClient, err := kube.NewDynamicClient(env.ControlPlane.Config)
			assert.NilError(t, err)

			err = project.Init("github.com/kharf/declcd/installation", env.TestProject, "1.0.0")
			assert.NilError(t, err)

			action := install.NewAction(kubeClient, client, env.TestProject)

			err = action.Install(
				ctx,
				install.Namespace(namespace),
				install.Branch(branch),
				install.Interval(intervalInSeconds),
				install.Name(name),
				install.URL(url),
				install.Token("aaaa"),
			)
			assert.NilError(t, err)

			tc.assertion(env, namespace)

			tc.post(env, action, namespace)
		})
	}
}

func defaultAssertion(t *testing.T, env projecttest.Environment, nsName string) {
	ctx := context.Background()
	var ns v1.Namespace
	err := env.TestKubeClient.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	assert.NilError(t, err)

	var statefulSet appsv1.StatefulSet
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: project.ControllerName, Namespace: project.ControllerNamespace},
		&statefulSet,
	)
	assert.NilError(t, err)

	var gitOpsProject gitops.GitOpsProject
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: name, Namespace: nsName},
		&gitOpsProject,
	)
	assert.NilError(t, err)

	var decKey v1.Secret
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: secret.K8sSecretName, Namespace: nsName},
		&decKey,
	)
	assert.NilError(t, err)

	projectFile, err := os.Open(filepath.Join(env.TestProject, "declcd/project.cue"))
	assert.NilError(t, err)

	projectContent, err := io.ReadAll(projectFile)
	assert.NilError(t, err)

	var projectBuf bytes.Buffer
	projectTmpl, err := template.New("").Parse(manifest.Project)

	assert.NilError(t, err)
	err = projectTmpl.Execute(&projectBuf, map[string]interface{}{
		"Name":                name,
		"Namespace":           namespace,
		"Branch":              branch,
		"PullIntervalSeconds": intervalInSeconds,
		"Url":                 url,
	})
	assert.NilError(t, err)

	assert.Equal(t, string(projectContent), projectBuf.String())
	_, err = os.Open(filepath.Join(env.TestProject, "secrets/recipients.cue"))
	assert.NilError(t, err)

	var vcsKey v1.Secret
	err = env.TestKubeClient.Get(
		ctx,
		types.NamespacedName{Name: vcs.K8sSecretName, Namespace: nsName},
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
		types.NamespacedName{Name: project.ControllerName, Namespace: project.ControllerNamespace},
		&service,
	)
	assert.NilError(t, err)
}
