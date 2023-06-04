package project

import (
	"os"

	"github.com/go-git/go-git/v5"
)

// A vcs Repository.
type Repository struct {
	path string
}

func NewRepository(path string) Repository {
	return Repository{path: path}
}

// RepositoryManager clones a remote vcs repository to a local path.
type RepositoryManager struct{}

func NewRepositoryManager() RepositoryManager {
	return RepositoryManager{}
}

// CloneOptions define configuration how to clone a vcs repository.
type CloneOptions struct {
	// Location of the remote vcs repository.
	url string
	// Location to where the vcs repository is loaded.
	targetPath string
}

type cloneOption = func(opt *CloneOptions)

// WithUrl provides a URL configuration for the clone function.
func WithUrl(url string) func(*CloneOptions) {
	return func(opt *CloneOptions) {
		opt.url = url
	}
}

// WithTarget provides a local path to where the vcs repository is cloned.
func WithTarget(path string) func(*CloneOptions) {
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
	if _, err := git.PlainClone(
		targetPath, false,
		&git.CloneOptions{URL: options.url, Progress: os.Stdout},
	); err != nil && err != git.ErrRepositoryAlreadyExists {
		return nil, err
	}

	repository := NewRepository(targetPath)
	return &repository, nil
}
