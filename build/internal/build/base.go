package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
)

var (
	// tool binaries
	localBin          = "bin"
	controllerGenPath = filepath.Join(localBin, "controller-gen")
	envTest           = filepath.Join(localBin, "setup-envtest")

	workDir = "/declcd"
	tmp     = "/tmp"
	declTmp = filepath.Join(tmp, "decl")
)

type stepResult struct {
	output    string
	container *dagger.Container
}

type step interface {
	name() string
	run(context.Context, *dagger.Container) (*stepResult, error)
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
		From("golang:1.21").
		WithDirectory(workDir, client.Host().Directory("."), dagger.ContainerWithDirectoryOpts{
			Include: []string{
				"cmd",
				"pkg",
				"internal",
				"api",
				"go.mod",
				"go.sum",
			},
		}).
		WithMountedCache("/go", goCache).
		WithWorkdir(workDir).
		WithoutEnvVariable("GOPATH").
		WithEnvVariable("GOBIN", filepath.Join(workDir, localBin)).
		WithEnvVariable("TMPDIR", tmp)

	latestContainer := base
	for _, step := range steps {
		stepResult, err := step.run(ctx, latestContainer)
		if err != nil {
			return err
		}
		fmt.Println(stepResult.output)
		fmt.Println("\033[32m", step.name()+" passed!")
		latestContainer = stepResult.container
	}

	return nil
}
