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

type stepOutput string

type step interface {
	name() string
	run(context.Context, *dagger.Container) (stepOutput, error)
}

func RunWith(steps ...step) error {
	ctx := context.Background()
	// initialize Dagger client
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout), dagger.WithWorkdir(".."))
	if err != nil {
		return err
	}
	defer client.Close()

	goCache := client.CacheVolume("go")

	base := client.Container().
		From("golang:1.20").
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
		WithEnvVariable("TMPDIR", tmp).
		WithExec([]string{"go", "install", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest"}).
		WithExec([]string{"go", "install", "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.11.3"}).
		WithExec([]string{controllerGenPath, "rbac:roleName=manager-role", "crd", "webhook", "paths=\"./...\"", "output:crd:artifacts:config=config/crd/bases"})

	for _, step := range steps {
		output, err := step.run(ctx, base)
		if err != nil {
			return err
		}
		fmt.Println(output)
		fmt.Println("\033[32m", step.name()+" passed!")
	}

	return nil
}
