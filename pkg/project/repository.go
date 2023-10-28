package project

import (
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-logr/logr"
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
type RepositoryManager struct {
	Log logr.Logger
}

func NewRepositoryManager(log logr.Logger) RepositoryManager {
	return RepositoryManager{
		Log: log,
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

// Load loads a remote vcs repository to a local path or opens it if it exists.
func (manager RepositoryManager) Load(opts ...loadOption) (*Repository, error) {
	options := &LoadOptions{}
	for _, opt := range opts {
		opt(options)
	}

	targetPath := options.targetPath
	logArgs := []interface{}{"remote url", options.url, "target path", targetPath}
	manager.Log.Info("Opening repository", logArgs...)
	gitRepository, err := git.PlainOpen(targetPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return nil, err
	}

	if err == git.ErrRepositoryNotExists {
		manager.Log.Info("Repository does not exist", logArgs...)
		manager.Log.Info("Cloning repository", logArgs...)
		gitRepository, err = git.PlainClone(
			targetPath, false,
			&git.CloneOptions{URL: options.url, Progress: os.Stdout},
		)
		if err != nil {
			return nil, err
		}
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
