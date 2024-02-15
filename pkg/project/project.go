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
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type nodeResult struct {
	node *component.Node
	err  error
}

// Load uses a given path to a project and returns the components as a directed acyclic dependency graph.
func (manager *Manager) Load(projectPath string) (*component.DependencyGraph, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	if _, err := os.Stat(projectPath); errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	resultChan := make(chan nodeResult)
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
						node, err := manager.buildNode(projectPath, relativePath)
						resultChan <- nodeResult{
							node: node,
							err:  err,
						}
						<-semaphore
					}()
				}
				return nil
			},
		)
		wg.Wait()
		if err != nil {
			resultChan <- nodeResult{
				node: nil,
				err:  fmt.Errorf("%w: %w", ErrLoadProject, err),
			}
		}
		close(resultChan)
	}()
	dag := component.NewDependencyGraph()
	for result := range resultChan {
		if result.err != nil {
			return nil, result.err
		}
		if err := dag.Insert(*result.node); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
		}
	}
	return &dag, nil
}

func (manager *Manager) buildNode(
	projectPath string,
	relativePath string,
) (*component.Node, error) {
	instance, err := manager.componentBuilder.Build(
		component.WithProjectRoot(projectPath),
		component.WithComponentPath(relativePath),
	)
	if err != nil {
		return nil, err
	}
	manifests := make([]kube.ManifestMetadata, 0, len(instance.Manifests))
	for _, unstructured := range instance.Manifests {
		manifests = append(manifests, kube.NewManifestMetadata(
			v1.TypeMeta{
				Kind:       unstructured.GetKind(),
				APIVersion: unstructured.GetAPIVersion(),
			},
			instance.ID,
			unstructured.GetName(),
			unstructured.GetNamespace(),
		))
	}
	releases := make([]helm.ReleaseMetadata, 0, len(instance.HelmReleases))
	for _, hr := range instance.HelmReleases {
		releases = append(releases, helm.NewReleaseMetadata(
			instance.ID,
			hr.Name,
			hr.Namespace,
		))
	}
	node := component.NewNode(
		instance.ID,
		relativePath,
		instance.Dependencies,
		manifests,
		releases,
	)
	return &node, nil
}
