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
	"path/filepath"
	"time"

	"dagger.io/dagger"
)

type Test struct {
	ID      string
	Package string
}

const TestAllArg = "./..."

var TestAll = Test{ID: "./..."}

var _ step = (*Test)(nil)

func (t Test) name() string {
	if t.ID != TestAllArg {
		return "Test " + t.ID
	}
	return "Tests"
}

func (t Test) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	testBase := request.container.
		WithExec(
			[]string{"go", "install", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest"},
		).
		WithExec([]string{envTest, "use", "1.27", "--bin-dir", localBin, "-p", "path"})

	apiServerPath, err := testBase.Stdout(ctx)
	if err != nil {
		return nil, err
	}
	prepareTest := testBase.WithWorkdir(workDir).
		WithEnvVariable("KUBEBUILDER_ASSETS", filepath.Join(workDir, apiServerPath)).
		WithEnvVariable("CACHEBUSTER", time.Now().String())

	var test *dagger.Container
	if t.ID == TestAllArg {
		test = prepareTest.
			WithExec([]string{"go", "test", "-v", TestAllArg, "-coverprofile", "cover.out"})
	} else {
		if t.Package == "" {
			test = prepareTest.
				WithExec([]string{"go", "test", "-v", "-run", t.ID})
		} else {
			test = prepareTest.
				WithExec([]string{"go", "test", "-v", "./" + t.Package, "-run", t.ID})
		}
	}
	return &stepResult{
		container: test.WithWorkdir(workDir),
	}, nil
}
