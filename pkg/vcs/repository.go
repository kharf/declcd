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
	"errors"
	"fmt"
	"net/http"
	"os"
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
	K8sSecretDataAuthType    = "auth"
	K8sSecretDataAuthTypeSSH = "ssh"
	SSHKey                   = "identity"
	SSHPubKey                = "identity.pub"
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

func (manager RepositoryManager) getAuthMethodFromSecret(
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
	remoteURL string,
	targetPath string,
	projectName string,
) (*Repository, error) {
	log := manager.log.WithValues(
		"remote url", remoteURL,
		"target path", targetPath,
	)

	projectName = strings.ToLower(projectName)
	secret, err := getAuthSecret(ctx, manager.kubeClient, manager.controllerNamespace, projectName)
	if err != nil {
		if k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
			return nil, err
		}
	}

	var authMethod transport.AuthMethod
	if secret != nil {
		authMethod, err = manager.getAuthMethodFromSecret(*secret)
		if err != nil {
			return nil, err
		}
	}

	log.V(1).Info("Opening repository")

	gitRepository, err := git.PlainOpen(targetPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return nil, err
	}

	if err == git.ErrRepositoryNotExists {
		log.V(1).Info("Repository not cloned yet")
		log.V(1).Info("Cloning repository")

		gitRepository, err = git.PlainClone(
			targetPath, false,
			&git.CloneOptions{
				URL:      remoteURL,
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
	projectName string,
) (*v1.Secret, error) {
	unstr := &unstructured.Unstructured{}
	unstr.SetName(SecretName(projectName))
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

func (config RepositoryConfigurator) CreateDeployKeyIfNotExists(
	ctx context.Context,
	fieldManager string,
	projectName string,
) error {
	projectName = strings.ToLower(projectName)

	sec, err := getAuthSecret(ctx, config.kubeClient, config.controllerNamespace, projectName)
	if err != nil {
		if k8sErrors.ReasonForError(err) != metav1.StatusReasonNotFound {
			return err
		}
	}

	if sec != nil {
		return nil
	}

	depKey, err := config.provider.CreateDeployKey(ctx, config.repoID, WithKeySuffix(projectName))
	if err != nil {
		return err
	}

	if depKey != nil {
		unstr := &unstructured.Unstructured{}
		unstr.SetName(SecretName(projectName))
		unstr.SetNamespace(config.controllerNamespace)
		unstr.SetKind("Secret")
		unstr.SetAPIVersion("v1")
		unstr.Object["data"] = map[string][]byte{
			SSHKey:                []byte(depKey.privateKeyOpenSSH),
			SSHPubKey:             []byte(depKey.publicKeyOpenSSH),
			K8sSecretDataAuthType: []byte(K8sSecretDataAuthTypeSSH),
		}

		err = config.kubeClient.Apply(ctx, unstr, fieldManager)
		if err != nil {
			return err
		}
	}

	return nil
}

func SecretName(projectName string) string {
	return fmt.Sprintf("%s-%s", "vcs-auth", projectName)
}
