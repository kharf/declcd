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
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v64/github"
)

type githubClient struct {
	client *gogithub.Client
}

func NewGithubClient(httpClient *http.Client, token string) *githubClient {
	client := gogithub.NewClient(httpClient).WithAuthToken(token)
	return &githubClient{
		client: client,
	}
}

var _ providerClient = (*githubClient)(nil)

func (g *githubClient) CreateDeployKey(
	ctx context.Context,
	id string,
	opts ...deployKeyOption,
) (*deployKey, error) {
	deployKey, err := genDeployKey(opts...)
	if err != nil {
		return nil, err
	}
	idSplit := strings.Split(id, "/")
	if len(idSplit) != 2 {
		return nil, fmt.Errorf(
			"%w: %s doesn't correspond to the owner/repo format",
			ErrRepositoryID,
			id,
		)
	}
	keyReqBody := gogithub.Key{
		Title: &deployKey.title,
		Key:   &deployKey.publicKeyOpenSSH,
	}
	_, _, err = g.client.Repositories.CreateKey(ctx, idSplit[0], idSplit[1], &keyReqBody)
	if err != nil {
		return nil, err
	}
	return deployKey, nil
}
