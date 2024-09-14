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
	"testing"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
)

func TestGithubClient_CreateDeployKey(t *testing.T) {
	server, client := gittest.MockGitProvider(
		t,
		"owner/repo",
		fmt.Sprintf("declcd-%s", `dev`),
		nil,
	)
	defer server.Close()
	githubClient := vcs.NewGithubClient(client, "abcd")
	ctx := context.Background()
	depKey, err := githubClient.CreateDeployKey(ctx, "owner/repo", vcs.WithKeySuffix("dev"))
	assert.NilError(t, err)
	assert.Assert(t, depKey != nil)
}

func TestGithubClient_CreatePullRequest(t *testing.T) {
	req := vcs.PullRequestRequest{
		RepoID:     "owner/repo",
		Title:      "update",
		Branch:     "new-update",
		BaseBranch: "main",
	}
	server, client := gittest.MockGitProvider(
		t,
		"owner/repo",
		fmt.Sprintf("declcd-%s", `dev`),
		[]vcs.PullRequestRequest{req},
	)
	defer server.Close()
	githubClient := vcs.NewGithubClient(client, "abcd")
	ctx := context.Background()
	err := githubClient.CreatePullRequest(ctx, req)
	assert.NilError(t, err)
}

func TestGitlabClient_CreateDeployKey(t *testing.T) {
	server, client := gittest.MockGitProvider(
		t,
		"owner/repo",
		fmt.Sprintf("declcd-%s", `dev`),
		nil,
	)
	defer server.Close()
	gitlabClient, err := vcs.NewGitlabClient(client, "abcd")
	assert.NilError(t, err)
	ctx := context.Background()
	depKey, err := gitlabClient.CreateDeployKey(ctx, "owner/repo", vcs.WithKeySuffix("dev"))
	assert.NilError(t, err)
	assert.Assert(t, depKey != nil)
}

func TestGitlabClient_CreatePullRequest(t *testing.T) {
	req := vcs.PullRequestRequest{
		RepoID:     "owner/repo",
		Title:      "update",
		Branch:     "new-update",
		BaseBranch: "main",
	}
	server, client := gittest.MockGitProvider(
		t,
		"owner/repo",
		fmt.Sprintf("declcd-%s", `dev`),
		[]vcs.PullRequestRequest{req},
	)
	defer server.Close()
	gitlabClient, err := vcs.NewGitlabClient(client, "abcd")
	assert.NilError(t, err)
	ctx := context.Background()
	err = gitlabClient.CreatePullRequest(ctx, req)
	assert.NilError(t, err)
}
