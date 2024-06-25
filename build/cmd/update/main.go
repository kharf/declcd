// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	_, err = client.Container().
		From("node:20.15.0-alpine").
		WithEnvVariable("LOG_LEVEL", "INFO").
		WithSecretVariable("RENOVATE_TOKEN", pat).
		WithEnvVariable("RENOVATE_REPOSITORIES", "kharf/declcd").
		WithExec([]string{"apk", "add", "--no-cache", "git", "go"}).
		WithExec([]string{"sh", "-c", "npm install -g renovate"}).
		WithExec([]string{"renovate"}).
		Sync(ctx)
	if err != nil {
		return err
	}
	return nil
}
