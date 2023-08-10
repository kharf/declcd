package main

import (
	"context"
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
	patString, err := pat.Plaintext(ctx)
	if err != nil {
		return err
	}

	updateContainer := client.Container().
		From("renovate/renovate:36.41").
		WithFile("renovate.json", client.Host().File("renovate.json")).
		WithEnvVariable("LOG_LEVEL", "debug").
		WithEnvVariable("RENOVATE_REPOSITORIES", "kharf/declcd").
		WithSecretVariable("RENOVATE_TOKEN", pat).
		WithEnvVariable("RENOVATE_CONFIG_FILE", "renovate.json").
		WithEnvVariable("GOPRIVATE", "github.com/kharf").
		WithEnvVariable("RENOVATE_HOST_RULES", "[{\"hostType\": \"github\", \"matchHost\": \"https://api.github.com/repos/kharf\","+
			"\"token\": \""+patString+"\"},{\"hostType\": \"go\", \"matchHost\": \"https://github.com/kharf\", \"token\": \""+patString+"\"},]").
		WithEnvVariable("CACHEBUSTER", time.Now().String())

	output, err := updateContainer.Stderr(ctx)
	if err != nil {
		return err
	}

	fmt.Println(output)
	return nil
}
