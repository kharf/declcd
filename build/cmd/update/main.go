package main

import (
	"context"
	"fmt"
	"os"

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
	pat := client.SetSecret("pat", os.Getenv("RENOVATE_TOKEN"))
	updateContainer := client.Container().
		From("renovate/renovate:37.315-full").
		WithDefaultArgs([]string{"kharf/declcd"}).
		WithEnvVariable("LOG_LEVEL", "INFO").
		WithSecretVariable("RENOVATE_TOKEN", pat)
	output, err := updateContainer.Stderr(ctx)
	if err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}
