package projecttest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	_ "github.com/kharf/declcd/test/workingdir"
	"github.com/otiai10/copy"
	"go.uber.org/zap/zapcore"
	"gotest.tools/v3/assert"
	ctrlZap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type Environment struct {
	ProjectManager project.Manager
	GitRepository  *gittest.LocalGitRepository
	TestRoot       string
	TestProject    string
	Log            logr.Logger
	*kubetest.Environment
	CUEModuleRegistry *ocitest.Registry
}

func (env *Environment) Stop() {
	if env.Environment != nil {
		env.Environment.Stop()
	}
	if env.CUEModuleRegistry != nil {
		env.CUEModuleRegistry.Close()
	}
	os.Setenv("CUE_REGISTRY", "")
}

type Option interface {
	Apply(opts *options)
}

type options struct {
	projectSource string
	kubeOpts      []kubetest.Option
}

type WithProjectSource string

var _ Option = (*WithProjectSource)(nil)

func (opt WithProjectSource) Apply(opts *options) {
	opts.projectSource = string(opt)
}

type withKubernetes []kubetest.Option

func WithKubernetes(opts ...kubetest.Option) withKubernetes {
	return opts
}

var _ Option = (*WithProjectSource)(nil)

func (opt withKubernetes) Apply(opts *options) {
	opts.kubeOpts = opt
}

func StartProjectEnv(t testing.TB, opts ...Option) Environment {
	options := options{
		projectSource: "simple",
	}
	for _, o := range opts {
		o.Apply(&options)
	}
	testRoot := filepath.Join(os.TempDir(), "declcd")
	err := os.MkdirAll(testRoot, 0700)
	assert.NilError(t, err)
	testProject, err := os.MkdirTemp(testRoot, "")
	assert.NilError(t, err)
	err = copy.Copy(filepath.Join("test/testdata", options.projectSource), testProject)
	assert.NilError(t, err)
	logOpts := ctrlZap.Options{
		Development: false,
		Level:       zapcore.Level(-1),
	}
	log := ctrlZap.New(ctrlZap.UseFlagOptions(&logOpts))
	repo, err := gittest.InitGitRepository(testProject)
	assert.NilError(t, err)
	kubeOpts := append(options.kubeOpts, kubetest.WithProject(repo, testProject, testRoot))
	env := kubetest.StartKubetestEnv(t, log, kubeOpts...)
	projectManager := project.NewManager(component.NewBuilder(), log, runtime.GOMAXPROCS(0))
	cueModuleRegistry := ocitest.StartCUERegistry(t, testRoot)
	return Environment{
		ProjectManager:    projectManager,
		GitRepository:     repo,
		TestRoot:          testRoot,
		TestProject:       testProject,
		Environment:       env,
		CUEModuleRegistry: cueModuleRegistry,
		Log:               log,
	}
}
