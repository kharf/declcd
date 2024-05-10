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
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	K8sSecretName            = "vcs-auth"
	K8sSecretDataAuthType    = "auth"
	K8sSecretDataAuthTypeSSH = "ssh"
	SSHKey                   = "identity"
	SSHPubKey                = "identity.pub"
	SSHKnownHosts            = "known_hosts"
)

// A vcs Repository.
type Repository struct {
	Path string
	pull PullFunc
}

type PullFunc = func() (string, error)

func NewRepository(path string, pull PullFunc) Repository {
	return Repository{Path: path, pull: pull}
}

func (repository *Repository) Pull() (string, error) {
	return repository.pull()
}

// RepositoryManager clones a remote vcs repository to a local path.
type RepositoryManager struct {
	controllerNamespace string
	kubeClient          kube.Client[unstructured.Unstructured]
	log                 logr.Logger
}

func NewRepositoryManager(
	controllerNamespace string,
	kubeClient kube.Client[unstructured.Unstructured],
	log logr.Logger,
) RepositoryManager {
	return RepositoryManager{
		log:                 log,
		controllerNamespace: controllerNamespace,
		kubeClient:          kubeClient,
	}
}

// LoadOptions define configuration how to load a vcs repository.
type LoadOptions struct {
	// Location of the remote vcs repository.
	// mandatory
	url string
	// Location to where the vcs repository is loaded.
	// mandatory
	targetPath string
}

type loadOption = func(opt *LoadOptions)

// WithUrl provides a URL configuration for the load function.
func WithUrl(url string) loadOption {
	return func(opt *LoadOptions) {
		opt.url = url
	}
}

// WithTarget provides a local path to where the vcs repository is cloned.
func WithTarget(path string) loadOption {
	return func(opt *LoadOptions) {
		opt.targetPath = path
	}
}

var (
	ErrAuthKeyNotFound = errors.New("VCS auth key secret not found")
)

func (manager RepositoryManager) getAuthMethodFromSecret(
	ctx context.Context,
	secret v1.Secret,
) (transport.AuthMethod, error) {
	var authMethod transport.AuthMethod
	switch string(secret.Data[K8sSecretDataAuthType]) {
	case "ssh":
		priv := secret.Data[SSHKey]
		public, err := ssh.NewPublicKeys("git", priv, "")
		if err != nil {
			return nil, err
		}
		authMethod = public
	}
	return authMethod, nil
}

// Load loads a remote vcs repository to a local path or opens it if it exists.
func (manager RepositoryManager) Load(
	ctx context.Context,
	opts ...loadOption,
) (*Repository, error) {
	options := &LoadOptions{}
	for _, opt := range opts {
		opt(options)
	}
	secret, err := getAuthSecret(ctx, manager.kubeClient, manager.controllerNamespace)
	if err != nil {
		if k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
			return nil, err
		}
	}
	if secret == nil {
		return nil, ErrAuthKeyNotFound
	}
	authMethod, err := manager.getAuthMethodFromSecret(ctx, *secret)
	if err != nil {
		return nil, err
	}
	targetPath := options.targetPath
	logArgs := []interface{}{"remote url", options.url, "target path", targetPath}
	manager.log.Info("Opening repository", logArgs...)
	gitRepository, err := git.PlainOpen(targetPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return nil, err
	}
	if err == git.ErrRepositoryNotExists {
		manager.log.Info("Repository not cloned yet", logArgs...)
		manager.log.Info("Cloning repository", logArgs...)
		switch authMethod.Name() {
		case ssh.PublicKeysName:
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			sshDir := filepath.Join(home, ".ssh")
			if err := os.MkdirAll(sshDir, 0700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(sshDir, SSHKnownHosts), secret.Data[SSHKnownHosts], 0600); err != nil {
				return nil, err
			}
		}
		gitRepository, err = git.PlainClone(
			targetPath, false,
			&git.CloneOptions{
				URL:      options.url,
				Progress: os.Stdout,
				Auth:     authMethod,
			},
		)
		if err != nil {
			return nil, err
		}
	}
	worktree, err := gitRepository.Worktree()
	if err != nil {
		return nil, err
	}
	pullFunc := func() (string, error) {
		err := worktree.Pull(&git.PullOptions{
			Auth: authMethod,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return "", err
		}
		ref, err := gitRepository.Head()
		if err != nil {
			return "", err
		}
		return ref.Hash().String(), nil
	}
	repository := NewRepository(targetPath, pullFunc)
	return &repository, nil
}

func getAuthSecret(
	ctx context.Context,
	kubeClient kube.Client[unstructured.Unstructured],
	controllerNamespace string,
) (*v1.Secret, error) {
	unstr := &unstructured.Unstructured{}
	unstr.SetName(K8sSecretName)
	unstr.SetNamespace(controllerNamespace)
	unstr.SetKind("Secret")
	unstr.SetAPIVersion("v1")
	unstr, err := kubeClient.Get(ctx, unstr)
	if err != nil {
		return nil, err
	}
	var sec v1.Secret
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.Object, &sec); err != nil {
		return nil, err
	}
	return &sec, nil
}

// RepositoryConfigurator is capable of setting up Declcd with a Git provider.
type RepositoryConfigurator struct {
	controllerNamespace string
	kubeClient          kube.Client[unstructured.Unstructured]
	provider            providerClient
	repoID              string
	token               string
}

var (
	ErrUnknownURLFormat = errors.New("Unknown git url format")
)

func NewRepositoryConfigurator(
	controllerNamespace string,
	kubeClient kube.Client[unstructured.Unstructured],
	httpClient *http.Client,
	url string,
	token string,
) (*RepositoryConfigurator, error) {
	var provider string
	var repoID string
	urlParts := strings.Split(url, "@")
	if len(urlParts) != 2 {
		provider = Generic
		repoID = url
	} else {
		providerIdParts := strings.Split(urlParts[1], ":")
		if len(providerIdParts) != 2 {
			return nil, fmt.Errorf("%s: expected one ':' in url '%s'", ErrUnknownURLFormat, url)
		}
		providerParts := strings.Split(providerIdParts[0], ".")
		if len(providerParts) != 2 {
			return nil, fmt.Errorf(
				"%s: expected one '.' in host '%s'",
				ErrUnknownURLFormat,
				providerIdParts[0],
			)
		}
		provider = providerParts[0]
		idSuffixParts := strings.Split(providerIdParts[1], ".")
		if len(idSuffixParts) != 2 {
			return nil, fmt.Errorf("%s: expected one '.' at end of url '%s'", ErrUnknownURLFormat, url)
		}
		repoID = idSuffixParts[0]
	}
	providerClient, err := getProviderClient(httpClient, provider, token)
	if err != nil {
		return nil, err
	}
	return &RepositoryConfigurator{
		controllerNamespace: controllerNamespace,
		kubeClient:          kubeClient,
		provider:            providerClient,
		repoID:              repoID,
		token:               token,
	}, nil
}

func (config RepositoryConfigurator) CreateDeployKeySecretIfNotExists(
	ctx context.Context,
	fieldManager string,
) error {
	sec, err := getAuthSecret(ctx, config.kubeClient, config.controllerNamespace)
	if err != nil {
		if k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
			return err
		}
	}
	if sec != nil {
		return nil
	}
	depKey, err := config.provider.CreateDeployKey(ctx, config.repoID)
	if err != nil {
		return err
	}
	if depKey != nil {
		unstr := &unstructured.Unstructured{}
		unstr.SetName(K8sSecretName)
		unstr.SetNamespace(config.controllerNamespace)
		unstr.SetKind("Secret")
		unstr.SetAPIVersion("v1")
		unstr.Object["data"] = map[string][]byte{
			SSHKey:                []byte(depKey.privateKeyOpenSSH),
			SSHPubKey:             []byte(depKey.publicKeyOpenSSH),
			K8sSecretDataAuthType: []byte(K8sSecretDataAuthTypeSSH),
			SSHKnownHosts:         []byte(config.provider.GetHostPublicSSHKey()),
		}
		err = config.kubeClient.Apply(ctx, unstr, fieldManager)
		if err != nil {
			return err
		}
	}
	return nil
}
