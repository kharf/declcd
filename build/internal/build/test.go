package build

import (
	"context"
	"path/filepath"

	"dagger.io/dagger"
)

type test func(ctx context.Context, container *dagger.Container) error

var _ step = (*test)(nil)
var Test test

func (_ test) name() string {
	return "Tests"
}

func (_ test) run(ctx context.Context, base *dagger.Container) (stepOutput, error) {
	testBase := base.
		WithExec([]string{envTest, "use", "1.26.1", "--bin-dir", localBin, "-p", "path"})

	apiServerPath, err := testBase.Stdout(ctx)
	if err != nil {
		return "", err
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

	testOutput, err := test.Stderr(ctx)
	if err != nil {
		return "", err
	}

	return stepOutput(testOutput), nil
}
