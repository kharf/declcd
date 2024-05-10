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

package vcs_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRepositoryManager_Load(t *testing.T) {
	testCases := []struct {
		name   string
		pre    func(localRepository string, remoteRepository *gittest.LocalGitRepository) (projecttest.Environment, *vcs.Repository)
		assert bool
	}{
		{
			name: "Clone",
			pre: func(localRepository string, remoteRepository *gittest.LocalGitRepository) (projecttest.Environment, *vcs.Repository) {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithKubernetes(
						kubetest.WithHelm(false, false, false),
						kubetest.WithVCSSSHKeyCreated(),
					),
				)
				defer env.Stop()
				repository, err := env.RepositoryManager.Load(
					env.Ctx,
					vcs.WithTarget(localRepository),
					vcs.WithUrl(remoteRepository.Directory),
				)
				assert.NilError(t, err)
				return env, repository
			},
			assert: true,
		},
		{
			name: "Open",
			pre: func(localRepository string, remoteRepository *gittest.LocalGitRepository) (projecttest.Environment, *vcs.Repository) {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithKubernetes(
						kubetest.WithHelm(false, false, false),
						kubetest.WithVCSSSHKeyCreated(),
					),
				)
				defer env.Stop()
				repository, err := env.RepositoryManager.Load(
					env.Ctx,
					vcs.WithTarget(localRepository),
					vcs.WithUrl(remoteRepository.Directory),
				)
				assert.NilError(t, err)
				repository, err = env.RepositoryManager.Load(
					env.Ctx,
					vcs.WithTarget(localRepository),
					vcs.WithUrl(remoteRepository.Directory),
				)
				assert.NilError(t, err)
				return env, repository
			},
			assert: true,
		},
		{
			name: "SecretMissing",
			pre: func(localRepository string, remoteRepository *gittest.LocalGitRepository) (projecttest.Environment, *vcs.Repository) {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithKubernetes(
						kubetest.WithHelm(false, false, false),
					),
				)
				defer env.Stop()
				repository, err := env.RepositoryManager.Load(
					env.Ctx,
					vcs.WithTarget(localRepository),
					vcs.WithUrl(remoteRepository.Directory),
				)
				assert.ErrorIs(t, err, vcs.ErrAuthKeyNotFound)
				return env, repository
			},
			assert: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			localRepository, err := os.MkdirTemp("", "")
			assert.NilError(t, err)
			defer os.RemoveAll(localRepository)
			remoteRepository, err := gittest.SetupGitRepository()
			assert.NilError(t, err)
			defer remoteRepository.Clean()
			_, repository := tc.pre(localRepository, remoteRepository)
			if tc.assert {
				dirInfo, err := os.Stat(repository.Path)
				assert.NilError(t, err)
				assert.Assert(t, dirInfo.IsDir())
				assert.Assert(t, repository.Path == localRepository)
				newFile := "test2"
				commitHash, err := remoteRepository.CommitNewFile(newFile, "second commit")
				assert.NilError(t, err)
				pulledCommitHash, err := repository.Pull()
				assert.NilError(t, err)
				assert.Equal(t, pulledCommitHash, commitHash)
				fileInfo, err := os.Stat(filepath.Join(localRepository, newFile))
				assert.NilError(t, err)
				assert.Assert(t, !fileInfo.IsDir())
				assert.Assert(t, fileInfo.Name() == newFile)
				home, err := os.UserHomeDir()
				assert.NilError(t, err)
				knownHostFile, err := os.Open(filepath.Join(home, ".ssh", vcs.SSHKnownHosts))
				assert.NilError(t, err)
				knownHostContent, err := io.ReadAll(knownHostFile)
				assert.NilError(t, err)
				assert.Equal(t, string(knownHostContent), vcs.GitHubSSHKey)
			}
		})
	}
}

func TestNewRepositoryConfigurator(t *testing.T) {
	ns := "test"
	testCases := []struct {
		name        string
		url         string
		expectedErr error
	}{
		{
			name:        "No@",
			url:         "github.com:kharf/declcd.git",
			expectedErr: nil,
		},
		{
			name:        "Multiple@",
			url:         "git@@github.com:kharf/declcd.git",
			expectedErr: nil,
		},
		{
			name: "Missing:",
			url:  "git@github.comkharf/declcd.git",
			expectedErr: fmt.Errorf(
				"%w: expected one ':' in url 'git@github.comkharf/declcd.git'",
				vcs.ErrUnknownURLFormat,
			),
		},
		{
			name: "Multiple:",
			url:  "git@github.com:kha:rf/declcd.git",
			expectedErr: fmt.Errorf(
				"%w: expected one ':' in url 'git@github.com:kha:rf/declcd.git'",
				vcs.ErrUnknownURLFormat,
			),
		},
		{
			name: "MissingDotInHost",
			url:  "git@githubcom:kharf/declcd.git",
			expectedErr: fmt.Errorf(
				"%w: expected one '.' in host 'githubcom'",
				vcs.ErrUnknownURLFormat,
			),
		},
		{
			name: "MultipleDotsInHost",
			url:  "git@gith.ub.com:kharf/declcd.git",
			expectedErr: fmt.Errorf(
				"%w: expected one '.' in host 'gith.ub.com'",
				vcs.ErrUnknownURLFormat,
			),
		},
		{
			name:        "UnknownProvider",
			url:         "git@gitthub.com:kharf/declcd.git",
			expectedErr: nil,
		},
		{
			name:        "GitLab",
			url:         "git@gitlab.com:kharf/declcd.git",
			expectedErr: nil,
		},
		{
			name:        "GitHub",
			url:         "git@github.com:kharf/declcd.git",
			expectedErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := vcs.NewRepositoryConfigurator(ns, &kube.DynamicClient{}, nil, tc.url, "abcd")
			if tc.expectedErr == nil {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.expectedErr.Error())
			}
		})
	}
}

func TestRepositoryConfigurator_CreateDeployKeySecretIfNotExists(t *testing.T) {
	server, client := gittest.MockGitProvider(t, vcs.GitHub)
	defer server.Close()
	ns := "test"
	testCases := []struct {
		name string
		pre  func() projecttest.Environment
		post func(env projecttest.Environment, sec corev1.Secret)
	}{
		{
			name: "NonExisting",
			pre: func() projecttest.Environment {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithProjectSource("empty"),
					projecttest.WithKubernetes(kubetest.WithHelm(false, false, false)),
				)
				configurator, err := vcs.NewRepositoryConfigurator(
					ns,
					env.DynamicTestKubeClient,
					client,
					"git@github.com:kharf/declcd.git",
					"abcd",
				)
				assert.NilError(t, err)
				err = configurator.CreateDeployKeySecretIfNotExists(env.Ctx, "manager")
				assert.NilError(t, err)
				return env
			},
			post: func(env projecttest.Environment, sec corev1.Secret) {},
		},
		{
			name: "Existing",
			pre: func() projecttest.Environment {
				env := projecttest.StartProjectEnv(t,
					projecttest.WithProjectSource("empty"),
					projecttest.WithKubernetes(kubetest.WithHelm(false, false, false)),
				)
				configurator, err := vcs.NewRepositoryConfigurator(
					ns,
					env.DynamicTestKubeClient,
					client,
					"git@github.com:kharf/declcd.git",
					"abcd",
				)
				assert.NilError(t, err)
				err = configurator.CreateDeployKeySecretIfNotExists(env.Ctx, "manager")
				assert.NilError(t, err)
				return env
			},
			post: func(env projecttest.Environment, sec corev1.Secret) {
				configurator, err := vcs.NewRepositoryConfigurator(
					ns,
					env.DynamicTestKubeClient,
					client,
					"git@github.com:kharf/declcd.git",
					"abcd",
				)
				assert.NilError(t, err)
				err = configurator.CreateDeployKeySecretIfNotExists(env.Ctx, "manager")
				assert.NilError(t, err)
				var sec2 corev1.Secret
				err = env.TestKubeClient.Get(
					env.Ctx,
					types.NamespacedName{Namespace: ns, Name: vcs.K8sSecretName},
					&sec2,
				)
				assert.NilError(t, err)
				key, _ := sec.Data[vcs.SSHKey]
				key2, _ := sec2.Data[vcs.SSHKey]
				assert.Equal(t, string(key2), string(key))
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.pre()
			defer env.Stop()
			var sec corev1.Secret
			err := env.TestKubeClient.Get(
				env.Ctx,
				types.NamespacedName{Namespace: ns, Name: vcs.K8sSecretName},
				&sec,
			)
			assert.NilError(t, err)
			key, _ := sec.Data[vcs.SSHKey]
			assert.Assert(t, strings.HasPrefix(string(key), "-----BEGIN OPENSSH PRIVATE KEY-----"))
			assert.Assert(t, strings.HasSuffix(string(key), "-----END OPENSSH PRIVATE KEY-----\n"))
			pubKey, _ := sec.Data[vcs.SSHPubKey]
			assert.Assert(t, strings.HasPrefix(string(pubKey), "ssh-ed25519 AAAA"))
			authType, _ := sec.Data[vcs.K8sSecretDataAuthType]
			assert.Equal(t, string(authType), vcs.K8sSecretDataAuthTypeSSH)
			knownHosts, _ := sec.Data[vcs.SSHKnownHosts]
			assert.Equal(t, string(knownHosts), vcs.GitHubSSHKey)
			tc.post(env, sec)
		})
	}
}
