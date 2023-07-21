package project

import (
	"os"

	"github.com/go-git/go-git/v5"
)

// A vcs Repository.
type Repository struct {
	Path string
	pull PullFunc
}

type PullFunc = func() error

func NewRepository(path string, pull PullFunc) Repository {
	return Repository{Path: path, pull: pull}
}

func (repository *Repository) Pull() error {
	return repository.pull()
}

// RepositoryManager clones a remote vcs repository to a local path.
type RepositoryManager struct{}

func NewRepositoryManager() RepositoryManager {
	return RepositoryManager{}
}

// CloneOptions define configuration how to clone a vcs repository.
type CloneOptions struct {
	// Location of the remote vcs repository.
	// mandatory
	url string
	// Location to where the vcs repository is loaded.
	// mandatory
	targetPath string
}

type cloneOption = func(opt *CloneOptions)

// WithUrl provides a URL configuration for the clone function.
func WithUrl(url string) cloneOption {
	return func(opt *CloneOptions) {
		opt.url = url
	}
}

// WithTarget provides a local path to where the vcs repository is cloned.
func WithTarget(path string) cloneOption {
	return func(opt *CloneOptions) {
		opt.targetPath = path
	}
}

// Clone loads a remote vcs repository to a local path.
func (manager RepositoryManager) Clone(opts ...cloneOption) (*Repository, error) {
	options := &CloneOptions{}
	for _, opt := range opts {
		opt(options)
	}

	targetPath := options.targetPath
	gitRepository, err := git.PlainClone(
		targetPath, false,
		&git.CloneOptions{URL: options.url, Progress: os.Stdout},
	)
	if err != nil && err != git.ErrRepositoryAlreadyExists {
		return nil, err
	}

	worktree, err := gitRepository.Worktree()
	if err != nil {
		return nil, err
	}

	pullFunc := func() error {
		err := worktree.Pull(&git.PullOptions{})
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	}

	repository := NewRepository(targetPath, pullFunc)
	return &repository, nil
}
