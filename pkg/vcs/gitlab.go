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

package vcs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	gogitlab "github.com/xanzy/go-gitlab"
)

type GitlabRepository struct {
	GenericRepository
	client gitlabClient
}

func (g *GitlabRepository) CreatePullRequest(title, desc, branch, targetBranch string) error {
	return g.client.CreatePullRequest(context.Background(),
		PullRequestRequest{
			RepoID:      g.RepoID(),
			Title:       title,
			Description: desc,
			Branch:      branch,
			BaseBranch:  targetBranch,
		},
	)
}

var _ Repository = (*GithubRepository)(nil)

type gitlabClient struct {
	client *gogitlab.Client
}

func (g *gitlabClient) CreatePullRequest(
	ctx context.Context,
	req PullRequestRequest,
) error {
	_, resp, err := g.client.MergeRequests.CreateMergeRequest(
		req.RepoID,
		&gogitlab.CreateMergeRequestOptions{
			Title:        &req.Title,
			Description:  &req.Description,
			SourceBranch: &req.Branch,
			TargetBranch: &req.BaseBranch,
		},
	)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			msg, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if strings.Contains(string(msg), "already exists") {
				return fmt.Errorf("%w: %s", ErrPRAlreadyExists, msg)
			}
		}

		return err
	}

	return nil
}

func (g *gitlabClient) CreateDeployKey(
	ctx context.Context,
	id string,
	opts ...deployKeyOption,
) (*deployKey, error) {
	deployKey, err := genDeployKey(opts...)
	if err != nil {
		return nil, err
	}
	canPush := true
	_, _, err = g.client.DeployKeys.AddDeployKey(id, &gogitlab.AddDeployKeyOptions{
		Title:   &deployKey.title,
		Key:     &deployKey.publicKeyOpenSSH,
		CanPush: &canPush,
	})
	if err != nil {
		return nil, err
	}
	return deployKey, nil
}

func NewGitlabClient(httpClient *http.Client, token string) (*gitlabClient, error) {
	client, err := gogitlab.NewClient(token, gogitlab.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	return &gitlabClient{
		client: client,
	}, nil
}

var _ providerClient = (*gitlabClient)(nil)
