package project

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	ErrMainComponentNotFound  = errors.New("main component not found")
	ErrLoadProject            = errors.New("could not load project")
	ProjectMainComponentPaths = []string{
		"/infra",
		"/apps",
	}
)

// Defines the CUE schema of decl's components.
const ComponentSchema = `
#component: {
	intervalSeconds: uint | *60
	manifests: [...]
}
`

const (
	ComponentFileName = "component.cue"
)

// Component defines the component's manifests and its reconciliation interval.
type Component struct {
	IntervalSeconds int                         `json:"intervalSeconds"`
	Manifests       []unstructured.Unstructured `json:"manifests"`
}

// MainDeclarativeComponent is an expected entry point for the project, containing all the declarative components.
type MainDeclarativeComponent struct {
	SubComponents []*SubDeclarativeComponent
}

func NewMainDeclarativeComponent(subComponents []*SubDeclarativeComponent) MainDeclarativeComponent {
	return MainDeclarativeComponent{SubComponents: subComponents}
}

// SubDeclarativeComponent is an entry point for containing all the declarative manifests.
type SubDeclarativeComponent struct {
	SubComponents []*SubDeclarativeComponent
	// Relative path to the project path.
	Path string
}

func NewSubDeclarativeComponent(subComponents []*SubDeclarativeComponent, path string) SubDeclarativeComponent {
	return SubDeclarativeComponent{SubComponents: subComponents, Path: path}
}

// FileSystem provides access to a declcd file system.
type FileSystem struct {
	fs.FS
	Root string
}

func NewFileSystem(fs fs.FS, root string) FileSystem {
	return FileSystem{FS: fs, Root: root}
}

// ProjectManager loads a declcd [Project] from given File System.
type ProjectManager struct {
	FS     FileSystem
	logger *zap.SugaredLogger
}

func NewProjectManager(fs FileSystem, logger *zap.SugaredLogger) ProjectManager {
	return ProjectManager{FS: fs, logger: logger}
}

// Load uses a given path to a project and loads it into a slice of [MainDeclarativeComponent]s.
func (p ProjectManager) Load(projectPath string) ([]MainDeclarativeComponent, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	mainDeclarativeComponents := make([]MainDeclarativeComponent, 0, len(ProjectMainComponentPaths))
	for _, mainComponentPath := range ProjectMainComponentPaths {
		fullMainComponentPath := projectPath + mainComponentPath
		if _, err := fs.Stat(p.FS, fullMainComponentPath); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: could not load %s", ErrMainComponentNotFound, fullMainComponentPath)
		}

		mainDeclarativeSubComponents := make([]*SubDeclarativeComponent, 0, 10)
		subComponentsByPath := make(map[string]*SubDeclarativeComponent)
		err := fs.WalkDir(p.FS, fullMainComponentPath, func(path string, dirEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			parentPath := strings.TrimSuffix(path, "/"+dirEntry.Name())
			if !dirEntry.IsDir() && parentPath == fullMainComponentPath {
				return nil
			}

			if dirEntry.IsDir() && path == fullMainComponentPath {
				return nil
			}

			parent := subComponentsByPath[parentPath]
			if dirEntry.IsDir() {
				componentFilePath := path + "/" + ComponentFileName
				if _, err := fs.Stat(p.FS, componentFilePath); errors.Is(err, fs.ErrNotExist) {
					p.logger.Infof("skipping directory %s, because no component.cue was found", dirEntry.Name())
					return filepath.SkipDir
				}
				p.logger.Infof("found component %s", path)
				relativePath, err := filepath.Rel(projectPath, path)
				if err != nil {
					return err
				}
				subDeclarativeComponent := &SubDeclarativeComponent{Path: relativePath}
				subComponentsByPath[path] = subDeclarativeComponent
				if parentPath != fullMainComponentPath {
					parent.SubComponents = append(parent.SubComponents, subDeclarativeComponent)
				} else {
					mainDeclarativeSubComponents = append(mainDeclarativeSubComponents, subDeclarativeComponent)
				}
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
		}

		mainDeclarativeComponents = append(mainDeclarativeComponents, NewMainDeclarativeComponent(mainDeclarativeSubComponents))
	}

	return mainDeclarativeComponents, nil
}
