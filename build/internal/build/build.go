package build

import (
	"context"

	"dagger.io/dagger"
)

type stepFunc = func(context.Context, *dagger.Container) error

type gen stepFunc

var _ step = (*gen)(nil)
var Gen gen

func (_ gen) name() string {
	return "Generate"
}

func (_ gen) run(ctx context.Context, base *dagger.Container) (*stepResult, error) {
	gen := base.
		WithExec([]string{"go", "install", "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.11.3"}).
		WithExec([]string{controllerGenPath, "rbac:roleName=manager-role", "crd", "webhook", "paths=\"./...\"", "output:crd:artifacts:config=config/crd/bases"})

	genOutput, err := gen.Stderr(ctx)
	if err != nil {
		return nil, err
	}

	return &stepResult{
		output:    genOutput,
		container: gen,
	}, nil
}

type build stepFunc

var _ step = (*build)(nil)
var Build build

func (_ build) name() string {
	return "Build"
}

func (_ build) run(ctx context.Context, base *dagger.Container) (*stepResult, error) {
	binary := "bin/manager"
	build := base.
		WithExec([]string{"go", "build", "-ldflags=-s -w", "-o", binary, "cmd/controller/main.go"})

	_, err := build.File(binary).Export(ctx, binary)
	if err != nil {
		return nil, err
	}

	buildOutput, err := build.Stderr(ctx)
	if err != nil {
		return nil, err
	}

	return &stepResult{
		output:    buildOutput,
		container: build,
	}, nil
}
