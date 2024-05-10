// Copyright 2024 Google LLC
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
	"net/http"

	gogitlab "github.com/xanzy/go-gitlab"
)

const (
	GitLabSSHKey = "gitlab.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAfuCHKVTjquxvt6CM6tdG4SLp1Btn/nOeHHE5UOzRdf"
)

type gitlabClient struct {
	client *gogitlab.Client
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

func (g *gitlabClient) CreateDeployKey(ctx context.Context, id string, opts ...deployKeyOption) (*deployKey, error) {
	deployKey, err := genDeployKey(opts...)
	if err != nil {
		return nil, err
	}
	_, _, err = g.client.DeployKeys.AddDeployKey(id, &gogitlab.AddDeployKeyOptions{
		Title: &deployKey.title,
		Key:   &deployKey.publicKeyOpenSSH,
	})
	if err != nil {
		return nil, err
	}
	return deployKey, nil
}

func (g *gitlabClient) GetHostPublicSSHKey() string {
	return GitLabSSHKey
}
