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
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/kube"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrPRAlreadyExists = errors.New("Pull-Request already exists")
)

type Repository interface {
	Path() string
	RepoID() string
	Pull() (string, error)
	Commit(file, message string) (string, error)
	SwitchBranch(branch string, create bool) error
	CurrentBranch() (string, error)
	Push(src, dst string) error
	CreatePullRequest(title, desc, src, dst string) error
}

type repositoryOption func(*repositoryOptions)

type repositoryOptions struct {
	provider   *Provider
	auth       Auth
	httpClient *http.Client
}

func WithProvider(provider Provider) repositoryOption {
	return func(o *repositoryOptions) {
		o.provider = &provider
	}
}

func WithAuth(auth Auth) repositoryOption {
	return func(o *repositoryOptions) {
		o.auth = auth
	}
}

func WithHTTPClient(client *http.Client) repositoryOption {
	return func(o *repositoryOptions) {
		o.httpClient = client
	}
}

func NewRepository(
	localTargetPath string,
	gitRepository *git.Repository,
	opts ...repositoryOption,
) (Repository, error) {
	options := &repositoryOptions{
		httpClient: http.DefaultClient,
	}

	for _, opt := range opts {
		opt(options)
	}

	remote, err := gitRepository.Remote(git.DefaultRemoteName)
	if err != nil {
		return nil, err
	}

	provider, repoID, err := ParseURL(remote.Config().URLs[0])
	if err != nil {
		return nil, err
	}

	if options.provider != nil {
		provider = *options.provider
	}

	auth := options.auth

	genericRepo := GenericRepository{
		path:          localTargetPath,
		gitRepository: gitRepository,
		auth:          options.auth,
		repoID:        repoID,
	}

	switch provider {
	case GitHub:
		client := NewGithubClient(options.httpClient, auth.Token)
		return &GithubRepository{
			GenericRepository: genericRepo,
			client:            *client,
		}, nil

	case GitLab:
		client, err := NewGitlabClient(options.httpClient, auth.Token)
		if err != nil {
			return nil, err
		}

		return &GitlabRepository{
			GenericRepository: genericRepo,
			client:            *client,
		}, nil
	}

	return &genericRepo, nil
}

type GenericRepository struct {
	path          string
	gitRepository *git.Repository
	auth          Auth
	repoID        string
}

func Duplicate(src, dst string) error {
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return err
	}

	return nil
}

func (g *GenericRepository) Path() string {
	return g.path
}

func (g *GenericRepository) RepoID() string {
	return g.repoID
}

func (g *GenericRepository) Commit(file string, message string) (string, error) {
	worktree, err := g.gitRepository.Worktree()
	if err != nil {
		return "", err
	}

	status, err := worktree.Status()
	if err != nil {
		return "", err
	}

	if status.IsClean() {
		head, err := g.gitRepository.Head()
		if err != nil {
			return "", err
		}

		return head.Hash().String(), nil
	}

	_, err = worktree.Add(file)
	if err != nil {
		return "", err
	}

	hash, err := worktree.Commit(
		message,
		&git.CommitOptions{
			Author: &object.Signature{
				Name: "declcd-bot",
				When: time.Now(),
			},
		},
	)
	if err != nil {
		return "", err
	}

	return hash.String(), nil
}

var ErrPullRequestNotSupported = errors.New("Pull-Request not supported")

func (g *GenericRepository) CreatePullRequest(title string, desc, src string, dst string) error {
	return ErrPullRequestNotSupported
}

func (g *GenericRepository) SwitchBranch(branch string, create bool) error {
	worktree, err := g.gitRepository.Worktree()
	if err != nil {
		return err
	}

	branchRef := plumbing.NewBranchReferenceName(branch)

	if err := worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Force:  true,
	}); err != nil {
		if !create {
			return err
		}

		return worktree.Checkout(&git.CheckoutOptions{
			Create: true,
			Branch: branchRef,
		})
	}

	return nil
}

func (g *GenericRepository) Pull() (string, error) {
	worktree, err := g.gitRepository.Worktree()
	if err != nil {
		return "", err
	}

	head, err := g.gitRepository.Head()
	if err != nil {
		return "", err
	}

	err = worktree.Pull(&git.PullOptions{
		Auth: g.auth.Method,
		// for whatever reason, this has to be specified at least with github, even though the docs say HEAD is used when not specified,
		// or the first run will result into "non-fast-forward update" and after that "already up-to-date".
		// not reproducible in tests
		ReferenceName: head.Name(),
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", err
	}

	head, err = g.gitRepository.Head()
	if err != nil {
		return "", err
	}

	return head.Hash().String(), nil
}

func (g *GenericRepository) Push(src, dst string) error {
	refSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", src, dst))
	return g.gitRepository.Push(&git.PushOptions{
		Auth:     g.auth.Method,
		RefSpecs: []config.RefSpec{refSpec},
	})
}

func (g *GenericRepository) CurrentBranch() (string, error) {
	head, err := g.gitRepository.Head()
	if err != nil {
		return "", err
	}

	return head.Name().Short(), nil
}

var _ Repository = (*GenericRepository)(nil)

func Open(
	path string,
	opts ...repositoryOption,
) (Repository, error) {
	gitRepository, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	return NewRepository(
		path, gitRepository, opts...,
	)
}

// RepositoryManager clones a remote vcs repository to a local path.
type RepositoryManager struct {
	controllerNamespace string
	kubeClient          kube.Client[unstructured.Unstructured, unstructured.Unstructured]
	log                 logr.Logger
}

func NewRepositoryManager(
	controllerNamespace string,
	kubeClient kube.Client[unstructured.Unstructured, unstructured.Unstructured],
	log logr.Logger,
) RepositoryManager {
	return RepositoryManager{
		log:                 log,
		controllerNamespace: controllerNamespace,
		kubeClient:          kubeClient,
	}
}

// Load clones a remote vcs repository to a local path or opens it if it exists.
func (manager *RepositoryManager) Load(
	ctx context.Context,
	remoteURL string,
	branch string,
	targetPath string,
	projectName string,
) (Repository, error) {
	log := manager.log.WithValues(
		"remote url", remoteURL,
		"branch", branch,
		"target path", targetPath,
	)

	projectName = strings.ToLower(projectName)

	auth, err := GetAuth(ctx, manager.kubeClient, manager.controllerNamespace, projectName)
	if err != nil {
		return nil, err
	}

	log.V(1).Info("Opening repository")

	repository, err := Open(targetPath, WithAuth(*auth))
	if err != nil && err != git.ErrRepositoryNotExists {
		return nil, err
	}

	if err == git.ErrRepositoryNotExists {
		log.V(1).Info("Repository not cloned yet")
		log.V(1).Info("Cloning repository")

		refName := plumbing.NewBranchReferenceName(branch)
		gitRepository, err := git.PlainClone(
			targetPath, false,
			&git.CloneOptions{
				URL:           remoteURL,
				Progress:      nil,
				Auth:          auth.Method,
				ReferenceName: refName,
			},
		)
		if err != nil {
			return nil, err
		}

		repository, err = NewRepository(targetPath, gitRepository, WithAuth(*auth))
		if err != nil {
			return nil, err
		}
	}

	if err := repository.SwitchBranch(branch, false); err != nil {
		return nil, err
	}

	return repository, nil
}

func (manager *RepositoryManager) LoadLocally(
	ctx context.Context,
	srcRepoPath string,
	targetPath string,
	projectName string,
) (Repository, error) {
	auth, err := GetAuth(ctx, manager.kubeClient, manager.controllerNamespace, projectName)
	if err != nil {
		return nil, err
	}

	newRepo, err := Open(targetPath, WithAuth(*auth))
	if err != nil && err != git.ErrRepositoryNotExists {
		return nil, err
	}

	if err == git.ErrRepositoryNotExists {
		if err := Duplicate(srcRepoPath, targetPath); err != nil {
			return nil, err
		}

		gitRepository, err := git.PlainOpen(targetPath)
		if err != nil {
			return nil, err
		}

		newRepo, err = NewRepository(targetPath, gitRepository, WithAuth(*auth))
		if err != nil {
			return nil, err
		}
	}

	return newRepo, nil
}

// RepositoryConfigurator is capable of setting up Declcd with a Git provider.
type RepositoryConfigurator struct {
	controllerNamespace string
	kubeClient          kube.Client[unstructured.Unstructured, unstructured.Unstructured]
	provider            providerClient
	repoID              string
	token               string
}

var (
	ErrUnknownURLFormat = errors.New("Unknown git url format")
)

func NewRepositoryConfigurator(
	controllerNamespace string,
	kubeClient kube.Client[unstructured.Unstructured, unstructured.Unstructured],
	httpClient *http.Client,
	url string,
	token string,
) (*RepositoryConfigurator, error) {
	provider, repoID, err := ParseURL(url)
	if err != nil {
		return nil, err
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

const DefaultRepoID = "none/none"

func ParseURL(url string) (Provider, string, error) {
	urlParts := strings.Split(url, "@")

	if len(urlParts) != 2 {
		return Generic, DefaultRepoID, nil
	}

	var provider, repoID string
	providerIdParts := strings.Split(urlParts[1], ":")
	if len(providerIdParts) != 2 {
		return "", "", fmt.Errorf("%s: expected one ':' in url '%s'", ErrUnknownURLFormat, url)
	}

	providerParts := strings.Split(providerIdParts[0], ".")
	if len(providerParts) != 2 {
		return "", "", fmt.Errorf(
			"%s: expected one '.' in host '%s'",
			ErrUnknownURLFormat,
			providerIdParts[0],
		)
	}

	provider = providerParts[0]
	idSuffixParts := strings.Split(providerIdParts[1], ".")
	if len(idSuffixParts) != 2 {
		return "", "", fmt.Errorf(
			"%s: expected one '.' at end of url '%s'",
			ErrUnknownURLFormat,
			url,
		)
	}

	repoID = idSuffixParts[0]

	if provider != GitHub && provider != GitLab {
		provider = Generic
	}

	return Provider(provider), repoID, nil
}

func (config RepositoryConfigurator) CreateDeployKeyIfNotExists(
	ctx context.Context,
	fieldManager string,
	projectName string,
	persistToken bool,
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
		return config.createAuthSecret(
			ctx,
			projectName,
			fieldManager,
			*depKey,
			persistToken,
		)
	}

	return nil
}

func SecretName(projectName string) string {
	return fmt.Sprintf("%s-%s", "vcs-auth", projectName)
}
