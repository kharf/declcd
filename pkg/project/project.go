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
}

func NewManager(componentBuilder component.Builder, log logr.Logger) Manager {
	return Manager{
		componentBuilder: componentBuilder,
		log:              log,
	}
}

// Load uses a given path to a project and returns the components as a directed acyclic dependency graph.
func (p Manager) Load(projectPath string) (*component.DependencyGraph, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	builder := p.componentBuilder
	if _, err := os.Stat(projectPath); errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	dag := component.NewDependencyGraph()
	err := filepath.WalkDir(projectPath, func(path string, dirEntry fs.DirEntry, err error) error {
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
			instance, err := builder.Build(component.WithProjectRoot(projectPath), component.WithComponentPath(relativePath))
			if err != nil {
				return err
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
			err = dag.Insert(component.NewNode(
				instance.ID,
				relativePath,
				instance.Dependencies,
				manifests,
				releases,
			))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
	}
	return &dag, nil
}
