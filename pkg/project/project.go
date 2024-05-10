// Copyright 2024 Google LLC
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

package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/secret"
	"golang.org/x/sync/errgroup"
)

var (
	ErrLoadProject = errors.New("Could not load project")
)

// Manager loads a declcd project and resolves the component dependency graph.
type Manager struct {
	componentBuilder component.Builder
	log              logr.Logger
	workerPoolSize   int
}

func NewManager(componentBuilder component.Builder, log logr.Logger, workerPoolSize int) Manager {
	return Manager{
		componentBuilder: componentBuilder,
		log:              log,
		workerPoolSize:   workerPoolSize,
	}
}

type instanceResult struct {
	instances []component.Instance
	err       error
}

// Load uses a given path to a project and returns the components as a directed acyclic dependency graph.
func (manager *Manager) Load(
	projectPath string,
) (*component.DependencyGraph, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	if _, err := os.Stat(projectPath); errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	resultChan := make(chan instanceResult)
	go func() {
		defer close(resultChan)
		eg := errgroup.Group{}
		eg.SetLimit(manager.workerPoolSize)
		err := filepath.WalkDir(
			projectPath,
			func(path string, dirEntry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if dirEntry.IsDir() {
					// TODO implement a dynamic way for ignoring directories
					if path == filepath.Join(projectPath, secret.SecretsStatePackage) ||
						path == filepath.Join(projectPath, "cue.mod") ||
						path == filepath.Join(projectPath, ".git") {
						return filepath.SkipDir
					}
					hasCUE := false
					entries, err := os.ReadDir(path)
					if err != nil {
						return err
					}
					for _, entry := range entries {
						if strings.HasSuffix(entry.Name(), ".cue") {
							hasCUE = true
							break
						}
					}
					if !hasCUE {
						return nil
					}
					relativePath, err := filepath.Rel(projectPath, path)
					if err != nil {
						return err
					}
					eg.Go(func() error {
						instances, err := manager.componentBuilder.Build(
							component.WithProjectRoot(projectPath),
							component.WithPackagePath(relativePath),
						)
						if err != nil {
							return err
						}
						resultChan <- instanceResult{
							instances: instances,
						}
						return nil
					})
				}
				return nil
			},
		)
		if err := eg.Wait(); err != nil {
			resultChan <- instanceResult{
				err: err,
			}
		}
		if err != nil {
			resultChan <- instanceResult{
				err: err,
			}
		}
	}()
	dag := component.NewDependencyGraph()
	for result := range resultChan {
		if result.err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, result.err)
		}
		if err := dag.Insert(result.instances...); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
		}
	}
	return &dag, nil
}
