package build

import (
	"context"
	"path/filepath"
	"time"

	"dagger.io/dagger"
)

type Bench struct {
	ID      string
	Package string
}

const BenchAllArg = "./..."

var BenchAll = Test{ID: "./..."}

var _ step = (*Bench)(nil)

func (t Bench) name() string {
	if t.ID != TestAllArg {
		return "Benchmark " + t.ID
	}
	return "Benchmarks"
}

func (t Bench) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	benchBase := request.container.
		WithExec(
			[]string{"go", "install", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest"},
		).
		WithExec([]string{envTest, "use", "1.26.1", "--bin-dir", localBin, "-p", "path"})
	apiServerPath, err := benchBase.Stdout(ctx)
	if err != nil {
		return nil, err
	}
	prepareBench := benchBase.WithWorkdir(workDir).
		WithEnvVariable("KUBEBUILDER_ASSETS", filepath.Join(workDir, apiServerPath)).
		WithEnvVariable("CACHEBUSTER", time.Now().String())
	var test *dagger.Container
	if t.ID == BenchAllArg {
		test = prepareBench.
			WithExec([]string{"go", "test", "-run=^$", "-bench=.", TestAllArg})
	} else {
		test = prepareBench.
			WithExec([]string{"go", "test", "-run=^$", "-count=5", "-cpu=1,2,4,8,16", "-bench=" + t.ID, "./" + t.Package})
	}
	return &stepResult{
		container: test.WithWorkdir(workDir),
	}, nil
}
