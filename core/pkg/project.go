package core

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/kharf/declcd/core/api"
)

var (
	ErrMainComponentNotFound  = errors.New("main component not found")
	projectMainComponentPaths = []string{
		"/apps",
		"/infra",
	}
)

// MainDeclarativeComponent is an expected entry point for the project, containing all the declarative manifests.
type MainDeclarativeComponent struct {
	entry api.Entry
}

func NewMainDeclarativeComponent(entry api.Entry) MainDeclarativeComponent {
	return MainDeclarativeComponent{entry: entry}
}

// Project is the declcd representation of the "GitOps" repository with all its declarative components.
type Project struct {
	mainComponents []MainDeclarativeComponent
}

func NewProject(mainComponents []MainDeclarativeComponent) Project {
	return Project{mainComponents: mainComponents}
}

// ProjectManager loads a declcd [Project] from given File System.
type ProjectManager struct {
	fs           fs.FS
	entryBuilder FileEntryBuilder
}

func NewProjectManager(fs fs.FS, entryBuilder FileEntryBuilder) ProjectManager {
	return ProjectManager{fs: fs, entryBuilder: entryBuilder}
}

// Load uses a given path to a project and loads it into a [Project].
func (p ProjectManager) Load(projectPath string) (*Project, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	mcs := make([]MainDeclarativeComponent, 0, len(projectMainComponentPaths))
	for _, mcp := range projectMainComponentPaths {
		efp := projectPath + mcp + "/entry.cue"
		if _, err := fs.Stat(p.fs, efp); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: could not load %s", ErrMainComponentNotFound, efp)
		}
		entry, err := p.entryBuilder.Build(efp)
		if err != nil {
			return nil, err
		}
		mcs = append(mcs, NewMainDeclarativeComponent(*entry))
	}

	proj := NewProject(mcs)
	return &proj, nil
}
