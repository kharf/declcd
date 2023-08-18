package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kharf/declcd/internal/gittest"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestRepositoryManager_Load(t *testing.T) {
	opts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	repositoryManager := NewRepositoryManager(log)
	remoteRepository, err := gittest.SetupGitRepository()
	assert.NilError(t, err)
	defer remoteRepository.Clean()
	localRepository, err := os.MkdirTemp("", "")
	assert.NilError(t, err)
	defer os.RemoveAll(localRepository)
	repository, err := repositoryManager.Load(WithTarget(localRepository), WithUrl(remoteRepository.Directory))
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

func TestRepositoryManager_Load_AlreadyExists(t *testing.T) {
	opts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))
	repositoryManager := NewRepositoryManager(log)
	remoteRepository, err := gittest.SetupGitRepository()
	assert.NilError(t, err)
	defer remoteRepository.Clean()
	localRepository, err := os.MkdirTemp("", "")
	assert.NilError(t, err)
	defer os.RemoveAll(localRepository)
	_, err = repositoryManager.Load(WithTarget(localRepository), WithUrl(remoteRepository.Directory))
	assert.NilError(t, err)
	repository, err := repositoryManager.Load(WithTarget(localRepository), WithUrl(remoteRepository.Directory))
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
