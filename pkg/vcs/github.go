package vcs

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v58/github"
)

const (
	GitHubSSHKey = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"
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

func (g *githubClient) CreateDeployKey(ctx context.Context, id string, opts ...deployKeyOption) (*deployKey, error) {
	deployKey, err := genDeployKey(opts...)
	if err != nil {
		return nil, err
	}
	idSplit := strings.Split(id, "/")
	if len(idSplit) != 2 {
		return nil, fmt.Errorf("%w: %s doesn't correspond to the owner/repo format", ErrRepositoryID, id)
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

func (g *githubClient) GetHostPublicSSHKey() string {
	return GitHubSSHKey
}
