package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
)

var (
	// tool binaries
	localBin      = "bin"
	envTest       = filepath.Join(localBin, "setup-envtest")
	controllerGen = filepath.Join(localBin, "controller-gen")
	workDir       = "/declcd"
	tmp           = "/tmp"
)

type step interface {
	name() string
	run(context.Context, stepRequest) (*stepResult, error)
}

type stepRequest struct {
	container *dagger.Container
	client    *dagger.Client
}

type stepResult struct {
	container *dagger.Container
}

func RunWith(steps ...step) error {
	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout), dagger.WithWorkdir(".."))
	if err != nil {
		return err
	}
	defer client.Close()
	goCache := client.CacheVolume("go")
	base := client.Container().
		From("golang:1.22.2-alpine").
		WithExec([]string{"apk", "add", "--no-cache", "git"}).
		WithExec([]string{"apk", "add", "--no-cache", "curl"}).
		WithDirectory(workDir, client.Host().Directory("."), dagger.ContainerWithDirectoryOpts{
			Include: []string{
				".git",
				".gitignore",
				".github",
				"cmd",
				"pkg",
				"internal",
				"schema",
				"test",
				"api",
				"go.mod",
				"go.sum",
				"Dockerfile",
				"build/cue.mod",
				"build/gen_tool.cue",
				"build/workflows.cue",
			},
		}).
		WithMountedCache("/go", goCache).
		WithWorkdir(workDir).
		WithEnvVariable("GOBIN", filepath.Join(workDir, localBin)).
		WithEnvVariable("CUE_EXPERIMENT", "modules").
		WithEnvVariable("TMPDIR", tmp)
	latestContainer := base
	for _, step := range steps {
		stepResult, err := step.run(ctx, stepRequest{client: client, container: latestContainer})
		if err != nil {
			return err
		}
		if stepResult.container != nil {
			output, err := stepResult.container.Stderr(ctx)
			if err != nil {
				return err
			}
			fmt.Println(output)
			latestContainer = stepResult.container
		}
		fmt.Println("\033[32m", strings.ToUpper(step.name())+" passed!")
	}

	return nil
}
