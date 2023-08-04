package projecttest

import (
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type LocalGitRepository struct {
	Worktree  *git.Worktree
	Directory string
}

func (r *LocalGitRepository) Clean() error {
	return os.RemoveAll(r.Directory)
}

func (r *LocalGitRepository) CommitFile(file string, message string) error {
	worktree := r.Worktree
	if _, err := worktree.Add(file); err != nil {
		return err
	}
	_, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})

	return err
}

func (r *LocalGitRepository) CommitNewFile(file string, message string) error {
	if err := os.WriteFile(filepath.Join(r.Directory, file), []byte{}, 0664); err != nil {
		return err
	}
	return r.CommitFile(file, message)
}

func SetupGitRepository() (*LocalGitRepository, error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}
	fileName := "test1"
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, err
	}
	worktree, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	localRepository := &LocalGitRepository{
		Worktree:  worktree,
		Directory: dir,
	}
	if err := localRepository.CommitNewFile(fileName, "first commit"); err != nil {
		return nil, err
	}

	return localRepository, nil
}

func OpenGitRepository(dir string) (*LocalGitRepository, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	worktree, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	localRepository := &LocalGitRepository{
		Worktree:  worktree,
		Directory: dir,
	}

	return localRepository, nil
}
