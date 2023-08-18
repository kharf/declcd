package projecttest

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/kubetest"
	"github.com/kharf/declcd/pkg/project"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type ProjectEnv struct {
	ProjectManager    project.ProjectManager
	RepositoryManager project.RepositoryManager
	Log               logr.Logger
	*kubetest.KubetestEnv
}

func (env *ProjectEnv) Stop() {
	env.KubetestEnv.Stop()
}

func StartProjectEnv(t *testing.T) ProjectEnv {
	env := kubetest.StartKubetestEnv(t)
	fs := os.DirFS(env.TestRoot)
	// TODO: replace with logr
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logger, err := zapConfig.Build()
	if err != nil {
		t.Fail()
	}
	sugarredLogger := logger.Sugar()
	opts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&opts))

	projectManager := project.NewProjectManager(project.FileSystem{FS: fs, Root: env.TestRoot}, sugarredLogger)
	repositoryManger := project.NewRepositoryManager(log)
	return ProjectEnv{
		ProjectManager:    projectManager,
		RepositoryManager: repositoryManger,
		KubetestEnv:       env,
		Log:               log,
	}
}
