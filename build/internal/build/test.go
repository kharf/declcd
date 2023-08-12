package build

import (
	"context"
	"path/filepath"
)

type test struct{}

var Test = test{}

var _ step = (*test)(nil)

func (_ test) name() string {
	return "Tests"
}

func (_ test) run(ctx context.Context, request stepRequest) (*stepResult, error) {
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
		WithWorkdir(workDir)

	test := prepareTest.
		WithEnvVariable("KUBEBUILDER_ASSETS", filepath.Join(workDir, apiServerPath)).
		WithExec([]string{"go", "test", "./...", "-coverprofile", "cover.out"})

	return &stepResult{
		container: test,
	}, nil
}
