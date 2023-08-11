package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"dagger.io/dagger"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		return err
	}
	defer client.Close()

	pat := client.SetSecret("pat", os.Getenv("DECL_PAT"))

	if err := os.Mkdir("tmp", 0755); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
	}

	updateContainer := client.Container().
		From("renovate/renovate:latest").
		WithEnvVariable("LOG_LEVEL", "DEBUG").
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithMountedDirectory("/tmp", client.Host().Directory("tmp"), dagger.ContainerWithMountedDirectoryOpts{Owner: "1000:0"}).
		WithFile("/usr/src/app/renovate.json", client.Host().File("renovate.json")).
		WithEnvVariable("RENOVATE_REPOSITORIES", "kharf/declcd").
		WithSecretVariable("RENOVATE_TOKEN", pat)

	output, err := updateContainer.Stderr(ctx)
	if err != nil {
		return err
	}

	fmt.Println(output)
	return nil
}
