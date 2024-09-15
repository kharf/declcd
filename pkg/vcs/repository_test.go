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
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
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
		name         string
		branch       string
		remoteBranch string
		withSecret   bool
		expectedErr  string
		post         func(kubernetes *kubetest.Environment, localRepository string, remoteRepository string)
	}{
		{
			name:       "Clone",
			branch:     "main",
			withSecret: true,
		},
		{
			name:       "Open",
			branch:     "main",
			withSecret: true,
			post: func(kubernetes *kubetest.Environment, localRepository string, remoteRepository string) {
				_, err := kubernetes.RepositoryManager.Load(
					context.Background(),
					remoteRepository,
					"main",
					localRepository,
					"open",
				)
				assert.NilError(t, err)
			},
		},
		{
			name:       "Switch-Branch",
			branch:     "main",
			withSecret: true,
			post: func(kubernetes *kubetest.Environment, localRepository string, remoteRepository string) {
				repository, err := vcs.Open(localRepository)
				assert.NilError(t, err)

				currentBranch, err := repository.CurrentBranch()
				assert.NilError(t, err)
				assert.Equal(t, currentBranch, "main")

				err = repository.SwitchBranch("dev", true)
				assert.NilError(t, err)

				currentBranch, err = repository.CurrentBranch()
				assert.NilError(t, err)
				assert.Equal(t, currentBranch, "dev")

				_, err = kubernetes.RepositoryManager.Load(
					context.Background(),
					remoteRepository,
					"main",
					localRepository,
					"open",
				)
				assert.NilError(t, err)

				currentBranch, err = repository.CurrentBranch()
				assert.NilError(t, err)
				assert.Equal(t, currentBranch, "main")
			},
		},
		{
			name:         "Branch-Not-Found",
			branch:       "feature",
			remoteBranch: "main",
			withSecret:   true,
			expectedErr:  "reference not found",
		},
		{
			name:       "No-Secret",
			branch:     "main",
			withSecret: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			localRepository := t.TempDir()
			if tc.remoteBranch == "" {
				tc.remoteBranch = tc.branch
			}
			remoteRepository, err := gittest.SetupGitRepository(t, tc.remoteBranch)
			assert.NilError(t, err)

			kubernetesOpts := projecttest.WithKubernetes()
			if tc.withSecret {
				kubernetesOpts = append(kubernetesOpts, kubetest.WithVCSAuthSecretFor("test"))
			}
			env := projecttest.Environment{}

			kubernetes := kubetest.StartKubetestEnv(t, env.Log, kubetest.WithEnabled(true))
			defer kubernetes.Stop()
			repository, err := kubernetes.RepositoryManager.Load(
				context.Background(),
				remoteRepository.Directory,
				tc.branch,
				localRepository,
				tc.name,
			)

			if tc.expectedErr == "" {
				assert.NilError(t, err)
				dirInfo, err := os.Stat(repository.Path())
				assert.NilError(t, err)
				assert.Assert(t, dirInfo.IsDir())
				assert.Assert(t, repository.Path() == localRepository)
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
			} else {
				assert.Error(t, err, tc.expectedErr)
			}

			if tc.post != nil {
				tc.post(kubernetes, localRepository, remoteRepository.Directory)
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
	ns := "test"
	testCases := []struct {
		name         string
		projectNames []string
		persistToken bool
		post         func(kubernetes *kubetest.Environment, sec corev1.Secret, client *http.Client)
	}{
		{
			name:         "Non-Existing",
			projectNames: []string{"non-existing"},
			persistToken: true,
			post:         func(kubernetes *kubetest.Environment, sec corev1.Secret, client *http.Client) {},
		},
		{
			name:         "Non-Existing-And-Disallow-Token-Persistence",
			projectNames: []string{"non-existing-and-disallow-token-persistence"},
			persistToken: false,
			post:         func(kubernetes *kubetest.Environment, sec corev1.Secret, client *http.Client) {},
		},
		{
			name:         "MultipleNonExisting",
			projectNames: []string{"a", "b"},
			post:         func(kubernetes *kubetest.Environment, sec corev1.Secret, client *http.Client) {},
		},
		{
			name:         "Existing",
			projectNames: []string{"existing"},
			post: func(kubernetes *kubetest.Environment, sec corev1.Secret, client *http.Client) {
				configurator, err := vcs.NewRepositoryConfigurator(
					ns,
					kubernetes.DynamicTestKubeClient.DynamicClient(),
					client,
					"git@github.com:owner/repo.git",
					"abcd",
				)
				assert.NilError(t, err)
				err = configurator.CreateDeployKeyIfNotExists(
					context.Background(),
					"manager",
					"existing",
					true,
				)
				assert.NilError(t, err)
				var sec2 corev1.Secret
				err = kubernetes.TestKubeClient.Get(
					context.Background(),
					types.NamespacedName{
						Namespace: ns,
						Name:      vcs.SecretName("existing"),
					},
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
			env := projecttest.Environment{}
			kubernetes := kubetest.StartKubetestEnv(t, env.Log, kubetest.WithEnabled(true))
			defer kubernetes.Stop()

			projectNames := tc.projectNames

			for _, projectName := range projectNames {
				server, client := gittest.MockGitProvider(
					t,
					"owner/repo",
					fmt.Sprintf("declcd-%s", projectName),
					nil,
					nil,
				)
				defer server.Close()

				token := "abcd"

				configurator, err := vcs.NewRepositoryConfigurator(
					ns,
					kubernetes.DynamicTestKubeClient.DynamicClient(),
					client,
					"git@github.com:owner/repo.git",
					token,
				)
				assert.NilError(t, err)
				err = configurator.CreateDeployKeyIfNotExists(
					context.Background(),
					"manager",
					projectName,
					tc.persistToken,
				)
				assert.NilError(t, err)

				var sec corev1.Secret
				err = kubernetes.TestKubeClient.Get(
					context.Background(),
					types.NamespacedName{
						Namespace: ns,
						Name:      vcs.SecretName(projectName),
					},
					&sec,
				)
				assert.NilError(t, err)
				key, _ := sec.Data[vcs.SSHKey]
				assert.Assert(
					t,
					strings.HasPrefix(string(key), "-----BEGIN OPENSSH PRIVATE KEY-----"),
				)
				assert.Assert(
					t,
					strings.HasSuffix(string(key), "-----END OPENSSH PRIVATE KEY-----\n"),
				)
				pubKey, _ := sec.Data[vcs.SSHPubKey]
				assert.Assert(t, strings.HasPrefix(string(pubKey), "ssh-ed25519 AAAA"))
				authType, _ := sec.Data[vcs.K8sSecretDataAuthType]
				assert.Equal(t, string(authType), vcs.K8sSecretDataAuthTypeSSH)

				if tc.persistToken {
					persistedToken, _ := sec.Data[vcs.Token]
					assert.Equal(t, string(persistedToken), token)
				} else {
					persistedToken, found := sec.Data[vcs.Token]
					assert.Assert(t, !found)
					assert.Equal(t, string(persistedToken), "")
				}

				tc.post(kubernetes, sec, client)
			}
		})
	}
}

func TestNewRepository(t *testing.T) {
	testCases := []struct {
		name         string
		url          string
		wantRepoID   string
		wantProvider vcs.Provider
		wantErr      string
	}{
		{
			name:         "GitHub",
			url:          "git@github.com:kharf/declcd.git",
			wantRepoID:   "kharf/declcd",
			wantProvider: vcs.GitHub,
		},
		{
			name:         "GitLab",
			url:          "git@gitlab.com:kharf/declcd.git",
			wantRepoID:   "kharf/declcd",
			wantProvider: vcs.GitLab,
		},
		{
			name:         "Generic",
			url:          "git@myscm.com:kharf/declcd.git",
			wantRepoID:   "kharf/declcd",
			wantProvider: vcs.Generic,
		},
		{
			name:         "Local",
			url:          "/tmp/repo/declcd",
			wantRepoID:   "none/none",
			wantProvider: vcs.Generic,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			err := os.WriteFile(filepath.Join(dir, "README.md"), []byte{}, 0666)
			assert.NilError(t, err)
			_, err = gittest.InitGitRepository(t, t.TempDir(), dir, "main")
			assert.NilError(t, err)
			gitRepository, err := git.PlainOpen(dir)
			assert.NilError(t, err)
			err = gitRepository.DeleteRemote(git.DefaultRemoteName)
			assert.NilError(t, err)
			_, err = gitRepository.CreateRemote(
				&config.RemoteConfig{Name: git.DefaultRemoteName, URLs: []string{tc.url}},
			)
			assert.NilError(t, err)

			repository, err := vcs.NewRepository("any", gitRepository)
			assert.NilError(t, err)

			if tc.wantErr == "" {
				assert.NilError(t, err)
				switch tc.wantProvider {
				case vcs.GitHub:
					_, ok := repository.(*vcs.GithubRepository)
					assert.Assert(t, ok)
				case vcs.GitLab:
					_, ok := repository.(*vcs.GitlabRepository)
					assert.Assert(t, ok)
				default:
					_, ok := repository.(*vcs.GenericRepository)
					assert.Assert(t, ok)
				}

				assert.Equal(t, repository.RepoID(), tc.wantRepoID)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
