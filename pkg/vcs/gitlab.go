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
