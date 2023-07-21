package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kharf/declcd/internal/projecttest"
	"gotest.tools/v3/assert"
)

func TestRepositoryManager_Clone(t *testing.T) {
	repositoryManager := NewRepositoryManager()
	remoteRepository, err := projecttest.SetupGitRepository()
	assert.NilError(t, err)
	defer remoteRepository.Clean()
	localRepository, err := os.MkdirTemp("", "")
	assert.NilError(t, err)
	defer os.RemoveAll(localRepository)
	repository, err := repositoryManager.Clone(WithTarget(localRepository), WithUrl(remoteRepository.Directory))
	assert.NilError(t, err)
	dirInfo, err := os.Stat(localRepository)
	assert.NilError(t, err)
	assert.Assert(t, dirInfo.IsDir())
	assert.Assert(t, repository.Path == localRepository)
	newFile := "test2"
	err = remoteRepository.CommitNewFile(newFile, "second commit")
	assert.NilError(t, err)
	err = repository.Pull()
	assert.NilError(t, err)
	fileInfo, err := os.Stat(filepath.Join(localRepository, newFile))
	assert.NilError(t, err)
	assert.Assert(t, !fileInfo.IsDir())
	assert.Assert(t, fileInfo.Name() == newFile)
}
