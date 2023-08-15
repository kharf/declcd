package build

import (
	"context"
)

type controllerGen struct{}

var ControllerGen = controllerGen{}

var _ step = (*controllerGen)(nil)

func (_ controllerGen) name() string {
	return "Generate Controller"
}

func (_ controllerGen) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	gen := request.container.
		WithExec([]string{"go", "install", "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.11.3"}).
		WithExec([]string{controllerGenPath, "rbac:roleName=manager-role", "crd", "webhook", "paths=\"./...\"", "output:crd:artifacts:config=config/crd/bases"})

	return &stepResult{
		container: gen,
	}, nil
}

type build struct{}

var Build = build{}

var _ step = (*build)(nil)

func (_ build) name() string {
	return "Build"
}

func (_ build) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	binary := "bin/manager"
	build := request.container.
		WithExec([]string{"go", "build", "-ldflags=-s -w", "-o", binary, "cmd/controller/main.go"})

	_, err := build.File(binary).Export(ctx, binary)
	if err != nil {
		return nil, err
	}

	return &stepResult{
		container: build,
	}, nil
}
