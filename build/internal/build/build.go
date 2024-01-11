package build

import (
	"context"
	"fmt"
	"os"
	"time"

	"dagger.io/dagger"
)

type build string

var (
	Tidy    = build("tidy")
	Build   = build("build")
	Publish = build("publish")
)

var _ step = (*build)(nil)

func (b build) name() string {
	return string(b)
}

func (b build) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	var build *dagger.Container
	switch b {
	case Build:
		var err error
		build, err = compile(ctx, "controller", request)
		if err != nil {
			return nil, err
		}
		build, err = compile(ctx, "cli", request)
		if err != nil {
			return nil, err
		}
	case Publish:
		token := request.client.SetSecret("token", os.Getenv("GITHUB_TOKEN"))
		ref, err := request.container.Directory(".").
			DockerBuild().
			WithRegistryAuth("ghcr.io", "kharf", token).
			WithLabel("org.opencontainers.image.title", "declcd").
			WithLabel("org.opencontainers.image.created", time.Now().String()).
			WithLabel("org.opencontainers.image.source", "https://github.com/kharf/declcd").
			Publish(ctx, "ghcr.io/kharf/declcd:latest")
		if err != nil {
			return nil, err
		}
		fmt.Printf("Published image to: %s\n", ref)
	case Tidy:
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

func compile(ctx context.Context, cmd string, request stepRequest) (*dagger.Container, error) {
	binary := fmt.Sprintf("bin/%s", cmd)
	build := request.container.
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-ldflags=-s -w", "-o", binary, fmt.Sprintf("cmd/%s/main.go", cmd)}).
		WithExec([]string{"chmod", "+x", binary})

	_, err := build.File(binary).Export(ctx, binary, dagger.FileExportOpts{AllowParentDirPath: true})
	if err != nil {
		return nil, err
	}
	return build, nil
}
