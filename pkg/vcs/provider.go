package vcs

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/ssh"
	cryptoSSH "golang.org/x/crypto/ssh"
)

var (
	ErrRepositoryID = errors.New("Unknown repository id")
)

type deployKeyOptions struct {
	keySuffix string
}

type deployKeyOption interface {
	apply(*deployKeyOptions)
}

type WithKeySuffix string

func (s WithKeySuffix) apply(opts *deployKeyOptions) {
	opts.keySuffix = string(s)
}

type providerClient interface {
	CreateDeployKey(ctx context.Context, repoID string, opts ...deployKeyOption) (*deployKey, error)
	GetHostPublicSSHKey() string
}

type Provider string

const (
	GitHub  = "github"
	GitLab  = "gitlab"
	Generic = "generic"
)

func getProviderClient(
	httpClient *http.Client,
	provider string,
	token string,
) (providerClient, error) {
	switch provider {
	case GitHub:
		client := NewGithubClient(httpClient, token)
		return client, nil
	case GitLab:
		client, err := NewGitlabClient(httpClient, token)
		if err != nil {
			return nil, err
		}
		return client, nil
	default:
		return &GenericProviderClient{}, nil
	}
}

type GenericProviderClient struct{}

var _ providerClient = (*GenericProviderClient)(nil)

func (*GenericProviderClient) CreateDeployKey(
	ctx context.Context,
	repoID string,
	opts ...deployKeyOption,
) (*deployKey, error) {
	return nil, nil
}

func (*GenericProviderClient) GetHostPublicSSHKey() string {
	return ""
}

type deployKey struct {
	title             string
	publicKeyOpenSSH  string
	privateKeyOpenSSH string
}

func genDeployKey(opts ...deployKeyOption) (*deployKey, error) {
	deployKeyOpts := &deployKeyOptions{
		keySuffix: "",
	}
	for _, o := range opts {
		o.apply(deployKeyOpts)
	}
	publicKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	privKeyPemBlock, err := cryptoSSH.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	if err := pem.Encode(&buf, privKeyPemBlock); err != nil {
		return nil, err
	}
	privKeyString := buf.String()
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	publicKeyString := fmt.Sprintf(
		"%s %s",
		sshPublicKey.Type(),
		base64.StdEncoding.EncodeToString(sshPublicKey.Marshal()),
	)
	title := "declcd"
	if deployKeyOpts.keySuffix != "" {
		title = fmt.Sprintf("%s-%s", title, deployKeyOpts.keySuffix)
	}
	return &deployKey{
		title:             title,
		publicKeyOpenSSH:  publicKeyString,
		privateKeyOpenSSH: privKeyString,
	}, nil
}
