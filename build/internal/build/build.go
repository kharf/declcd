package build

import (
	"context"

	"dagger.io/dagger"
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
		WithExec([]string{controllerGenPath, "rbac:roleName=manager-role", "crd", "webhook", "paths=\"./...\"", "output:crd:artifacts:config=config/crd/bases"}).
		WithExec([]string{controllerGenPath, "object:headerFile=\"hack/boilerplate.go.txt\"", "paths=\"./...\""})

	apiDir := "api/"
	_, err := gen.Directory(apiDir).Export(ctx, apiDir)
	if err != nil {
		return nil, err
	}
	config := "config/"
	_, err = gen.Directory(config).Export(ctx, config)
	if err != nil {
		return nil, err
	}
	return &stepResult{
		container: gen,
	}, nil
}

type build string

var (
	Tidy  = build("tidy")
	Build = build("build")
)

var _ step = (*build)(nil)

func (b build) name() string {
	return string(b)
}

func (b build) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	var build *dagger.Container

	if b == Build {
		binary := "bin/manager"
		build = request.container.
			WithExec([]string{"go", "build", "-ldflags=-s -w", "-o", binary, "cmd/controller/main.go"})

		_, err := build.File(binary).Export(ctx, binary)
		if err != nil {
			return nil, err
		}
	} else {
		sum := "go.sum"
		mod := "go.mod"
		build = request.container.
			WithExec([]string{"go", "mod", "tidy"})

		_, err := build.File(sum).Export(ctx, sum)
		if err != nil {
			return nil, err
		}

		_, err = build.File(mod).Export(ctx, mod)
		if err != nil {
			return nil, err
		}
	}

	return &stepResult{
		container: build,
	}, nil
}
