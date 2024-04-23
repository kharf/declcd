package build

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"dagger.io/dagger"
)

var (
	Build = func(version string) build {
		return build{
			bType:   "build",
			version: version,
		}
	}
	Publish = func(version string) build {
		return build{
			bType:   "publish",
			version: version,
		}
	}
	Tidy = build{
		bType: "tidy",
	}
)

type build struct {
	bType   string
	version string
}

var _ step = (*build)(nil)

func (b build) name() string {
	return b.bType
}

type distribution struct {
	os         string
	arch       string
	fileEnding string
}

func (dist distribution) binaryName() string {
	return fmt.Sprintf("%s-%s-%s%s", "declcd", dist.os, dist.arch, dist.fileEnding)
}

// when changed, the renovate customManager has also to be updated.
var cueDep = "cuelang.org/go/cmd/cue@v0.8.1"

func (b build) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	var prefixedVersion string
	var version string
	if !strings.HasPrefix(b.version, "v") {
		prefixedVersion = "v" + b.version
		version = b.version
	} else {
		prefixedVersion = b.version
		version, _ = strings.CutPrefix(b.version, "v")
	}
	cliDistributions := []distribution{
		{
			os:   "linux",
			arch: "amd64",
		},
		{
			os:         "windows",
			arch:       "amd64",
			fileEnding: ".exe",
		},
		{
			os:   "darwin",
			arch: "amd64",
		},
		{
			os:   "darwin",
			arch: "arm64",
		},
	}
	var build *dagger.Container
	switch b.bType {
	case "build":
		var err error
		build, err = compile(ctx, "controller", "controller", prefixedVersion, request.container)
		if err != nil {
			return nil, err
		}
		for _, dist := range cliDistributions {
			build = build.
				WithEnvVariable("GOOS", dist.os).
				WithEnvVariable("GOARCH", dist.arch)
			build, err = compile(
				ctx,
				"cli",
				dist.binaryName(),
				version,
				build,
			)
			if err != nil {
				return nil, err
			}
		}
	case "publish":
		token := request.client.SetSecret("token", os.Getenv("GITHUB_TOKEN"))
		tokenPlaintext, err := token.Plaintext(ctx)
		if err != nil {
			return nil, err
		}
		ref, err := request.container.
			Directory(".").
			DockerBuild().
			WithRegistryAuth("ghcr.io", "kharf", token).
			WithLabel("org.opencontainers.image.title", "declcd").
			WithLabel("org.opencontainers.image.created", time.Now().String()).
			WithLabel("org.opencontainers.image.source", "https://github.com/kharf/declcd").
			Publish(ctx, "ghcr.io/kharf/declcd:"+version)
		if err != nil {
			return nil, err
		}
		fmt.Printf("Published image to: %s\n", ref)
		var artifacts []string
		for _, dist := range cliDistributions {
			artifacts = append(artifacts, "bin/"+dist.binaryName())
		}
		build = request.container.
			WithoutEnvVariable("GOOS").
			WithoutEnvVariable("GOARCH").
			WithExec([]string{"go", "install", cueDep}).
			WithEnvVariable("CUE_EXPERIMENT", "modules").
			WithEnvVariable("CUE_REGISTRY", "ghcr.io/kharf").
			WithWorkdir("schema").
			WithExec([]string{"docker", "login", "ghcr.io", "-u", "kharf", "-p", tokenPlaintext}).
			WithExec([]string{"../bin/cue", "mod", "publish", prefixedVersion}).
			WithWorkdir(workDir).
			WithExec(
				[]string{
					"curl",
					"-L",
					"https://github.com/kharf/monoreleaser/releases/download/v0.0.15/monoreleaser-linux-amd64",
					"--output",
					"monoreleaser",
				},
			).
			WithExec([]string{"chmod", "+x", "monoreleaser"}).
			WithSecretVariable("MR_GITHUB_TOKEN", token).
			WithExec([]string{"./monoreleaser", "release", ".", prefixedVersion, "--artifacts=" + strings.Join(artifacts, ",")})
	case "tidy":
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

func compile(
	ctx context.Context,
	cmd string,
	binaryName string,
	version string,
	container *dagger.Container,
) (*dagger.Container, error) {
	binary := fmt.Sprintf("bin/%s", binaryName)
	build := container.
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-ldflags= -X main.Version=" + version + " -s" + " -w", "-o", binary, fmt.Sprintf("cmd/%s/main.go", cmd)}).
		WithExec([]string{"chmod", "+x", binary})
	_, err := build.File(binary).
		Export(ctx, binary, dagger.FileExportOpts{AllowParentDirPath: false})
	if err != nil {
		return nil, err
	}
	return build, nil
}

var ApiGen apigen = "apigen"

type apigen string

var _ step = (*apigen)(nil)

func (g apigen) name() string {
	return string(g)
}

// when changed, the renovate customManager has also to be updated.
var controllerGenDep = "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0"

func (g apigen) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	gen := request.container.
		WithExec(
			[]string{"go", "install", controllerGenDep},
		).
		WithExec([]string{"go", "install", cueDep}).
		WithExec([]string{controllerGen, "crd", "paths=./api/v1/...", "output:crd:artifacts:config=internal/manifest"}).
		WithExec([]string{"bin/cue", "import", "-f", "-o", "internal/manifest/crd.cue", "internal/manifest/gitops.declcd.io_gitopsprojects.yaml", "-l", "_crd:", "-p", "declcd"})
	_, err := gen.File("internal/manifest/crd.cue").
		Export(ctx, "internal/manifest/crd.cue", dagger.FileExportOpts{AllowParentDirPath: false})
	if err != nil {
		return nil, err
	}
	return &stepResult{
		container: gen,
	}, nil
}
