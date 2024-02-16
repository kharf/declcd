package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
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
func (manager *Manager) Load(projectPath string) (*component.DependencyGraph, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	if _, err := os.Stat(projectPath); errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	resultChan := make(chan instanceResult)
	go func() {
		wg := sync.WaitGroup{}
		semaphore := make(chan struct{}, manager.workerPoolSize)
		err := filepath.WalkDir(
			projectPath,
			func(path string, dirEntry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if dirEntry.IsDir() {
					componentFilePath := filepath.Join(path, component.FileName)
					if _, err := os.Stat(componentFilePath); errors.Is(err, fs.ErrNotExist) {
						return nil
					}
					relativePath, err := filepath.Rel(projectPath, path)
					if err != nil {
						return err
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						semaphore <- struct{}{}
						instances, err := manager.componentBuilder.Build(
							component.WithProjectRoot(projectPath),
							component.WithComponentPath(relativePath),
						)
						resultChan <- instanceResult{
							instances: instances,
							err:       err,
						}
						<-semaphore
					}()
				}
				return nil
			},
		)
		wg.Wait()
		if err != nil {
			resultChan <- instanceResult{
				instances: []component.Instance{},
				err:       fmt.Errorf("%w: %w", ErrLoadProject, err),
			}
		}
		close(resultChan)
	}()
	dag := component.NewDependencyGraph()
	for result := range resultChan {
		if result.err != nil {
			return nil, result.err
		}
		if err := dag.Insert(result.instances...); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
		}
	}
	return &dag, nil
}
