package build

import (
	"context"
	"path/filepath"

	"dagger.io/dagger"
)

type Test string

const TestAllArg = "./..."

var TestAll = Test(TestAllArg)

var _ step = (*Test)(nil)

func (t Test) name() string {
	if t != TestAll {
		return "Test " + string(t)
	}
	return "Tests"
}

func (t Test) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	testBase := request.container.
		WithExec([]string{"go", "install", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest"}).
		WithExec([]string{envTest, "use", "1.26.1", "--bin-dir", localBin, "-p", "path"})

	apiServerPath, err := testBase.Stdout(ctx)
	if err != nil {
		return nil, err
	}

	prepareTest := testBase.WithExec([]string{"mkdir", "-p", declTmp}).
		WithExec([]string{"cp", "internal/controller/testdata/controllertest", "-r", declTmp}).
		WithWorkdir(filepath.Join(declTmp, "controllertest")).
		WithExec([]string{"git", "init", "."}).
		WithExec([]string{"git", "config", "user.email", "test@test.com"}).
		WithExec([]string{"git", "config", "user.name", "test"}).
		WithExec([]string{"git", "add", "."}).
		WithExec([]string{"git", "commit", "-m", "\"init\""}).
		WithWorkdir(workDir).
		WithEnvVariable("KUBEBUILDER_ASSETS", filepath.Join(workDir, apiServerPath))

	var test *dagger.Container
	if t == TestAll {
		test = prepareTest.
			WithExec([]string{"go", "test", TestAllArg, "-coverprofile", "cover.out"})
	} else {
		test = prepareTest.
			WithExec([]string{"go", "test", "-run", string(t)})
	}

	return &stepResult{
		container: test,
	}, nil
}
