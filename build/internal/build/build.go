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

package build

import (
	"context"
	"os"

	"dagger.io/dagger"
)

var ApiGen apigen = "apigen"

type apigen string

var _ step = (*apigen)(nil)

func (g apigen) name() string {
	return string(g)
}

// when changed, the renovate customManager has also to be updated.
var controllerGenDep = "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0"

// when changed, the renovate customManager has also to be updated.
var cueDep = "cuelang.org/go/cmd/cue@v0.8.2"

func (g apigen) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	gen := request.container.
		WithExec(
			[]string{"go", "install", controllerGenDep},
		).
		WithExec([]string{"go", "install", cueDep}).
		WithExec([]string{controllerGen, "crd", "paths=./api/v1beta1/...", "output:crd:artifacts:config=internal/manifest"}).
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

type Publish string

var _ step = (*Publish)(nil)

func (p Publish) name() string {
	return "publish"
}

// when changed, the renovate customManager has also to be updated.
var goreleaserDep = "github.com/goreleaser/goreleaser@v1.26.1"

func (p Publish) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	token := request.client.SetSecret("token", os.Getenv("GITHUB_TOKEN"))
	dockerSock := os.Getenv("DOCKER_SOCK")
	gen := request.container.
		WithUnixSocket(
			"/var/run/docker.sock",
			request.client.Host().UnixSocket(dockerSock),
		).
		WithSecretVariable("GITHUB_TOKEN", token).
		WithExec([]string{"go", "install", goreleaserDep}).
		WithExec(
			[]string{
				"sh",
				"-c",
				`git config --global url.https://kharf:$GITHUB_TOKEN@github.com/kharf/declcd.git.insteadOf git@github.com:kharf/declcd.git`,
			},
		).
		WithExec([]string{"git", "tag", string(p)}).
		WithExec([]string{"git", "push", "origin", string(p)}).
		WithExec([]string{"bin/goreleaser", "release", "--clean", "--skip=validate"})
	return &stepResult{
		container: gen,
	}, nil
}
