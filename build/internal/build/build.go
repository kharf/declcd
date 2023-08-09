package build

import (
	"context"

	"dagger.io/dagger"
)

type build func(context.Context, *dagger.Container) error

var _ step = (*build)(nil)
var Build build

func (_ build) name() string {
	return "Build"
}

func (_ build) run(ctx context.Context, base *dagger.Container) (stepOutput, error) {
	binary := "bin/manager"
	build := base.
		WithExec([]string{"go", "build", "-ldflags=-s -w", "-o", binary, "cmd/controller/main.go"})

	_, err := build.File(binary).Export(ctx, binary)
	if err != nil {
		return "", err
	}

	buildOutput, err := build.Stderr(ctx)
	if err != nil {
		return "", err
	}

	return stepOutput(buildOutput), nil
}
