package projecttest

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type ProjectEnv struct {
	ProjectManager    project.ProjectManager
	RepositoryManager project.RepositoryManager
	InventoryManager  inventory.Manager
	GarbageCollector  garbage.Collector
	Log               logr.Logger
	*kubetest.KubetestEnv
}

func (env *ProjectEnv) Stop() {
	env.KubetestEnv.Stop()
}

func StartProjectEnv(t *testing.T, kubeOpts ...kubetest.Option) ProjectEnv {
	env := kubetest.StartKubetestEnv(t, kubeOpts...)
	fs := os.DirFS(env.TestRoot)
	// TODO: replace with logr
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	logger, err := zapConfig.Build()
	assert.NilError(t, err)
	sugarredLogger := logger.Sugar()
	logOpts := ctrlZap.Options{
		Development: true,
		Level:       zapcore.Level(-3),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))

	client, err := kube.NewClient(env.ControlPlane.Config)
	assert.NilError(t, err)
	inventoryPath, err := os.MkdirTemp(env.TestRoot, "inventory-*")
	assert.NilError(t, err)
	invManager := inventory.Manager{
		Log:  log,
		Path: inventoryPath,
	}

	gc := garbage.Collector{
		Log:              log,
		Client:           client,
		InventoryManager: invManager,
		HelmConfig:       env.HelmEnv.HelmConfig,
	}

	projectManager := project.NewProjectManager(project.FileSystem{FS: fs, Root: env.TestRoot}, sugarredLogger)
	repositoryManger := project.NewRepositoryManager(log)
	return ProjectEnv{
		ProjectManager:    projectManager,
		RepositoryManager: repositoryManger,
		KubetestEnv:       env,
		InventoryManager:  invManager,
		GarbageCollector:  gc,
		Log:               log,
	}
}
