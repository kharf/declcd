package vcs_test

import (
	"context"
	"testing"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
)

func TestGithubClient_CreateDeployKey(t *testing.T) {
	server, client := gittest.MockGitProvider(t, vcs.GitHub)
	defer server.Close()
	githubClient := vcs.NewGithubClient(client, "abcd")
	ctx := context.Background()
	depKey, err := githubClient.CreateDeployKey(ctx, "owner/repo", vcs.WithKeySuffix("dev"))
	assert.NilError(t, err)
	assert.Assert(t, depKey != nil)
}

func TestGitlabClient_CreateDeployKey(t *testing.T) {
	server, client := gittest.MockGitProvider(t, vcs.GitLab)
	defer server.Close()
	gitlabClient, err := vcs.NewGitlabClient(client, "abcd")
	assert.NilError(t, err)
	ctx := context.Background()
	depKey, err := gitlabClient.CreateDeployKey(ctx, "owner/repo", vcs.WithKeySuffix("dev"))
	assert.NilError(t, err)
	assert.Assert(t, depKey != nil)
}
