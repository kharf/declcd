package core

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kharf/declcd/core/api"
)

var (
	ErrMainComponentNotFound  = errors.New("main component not found")
	ErrLoadProject            = errors.New("could not load project")
	projectMainComponentPaths = []string{
		"/apps",
		"/infra",
	}
)

// MainDeclarativeComponent is an expected entry point for the project, containing all the declarative components.
type MainDeclarativeComponent struct {
	entry         api.Entry
	subComponents []*SubDeclarativeComponent
}

func NewMainDeclarativeComponent(entry api.Entry, subComponents []*SubDeclarativeComponent) MainDeclarativeComponent {
	return MainDeclarativeComponent{entry: entry, subComponents: subComponents}
}

// SubDeclarativeComponent is an entry point for containing all the declarative manifests.
type SubDeclarativeComponent struct {
	entry         api.Entry
	subComponents []*SubDeclarativeComponent
	manifests     []*Manifest
}

func NewSubDeclarativeComponent(entry api.Entry, subComponents []*SubDeclarativeComponent, manifests []*Manifest) SubDeclarativeComponent {
	return SubDeclarativeComponent{entry: entry, subComponents: subComponents, manifests: manifests}
}

type Manifest struct {
	name string
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
	logger       *zap.SugaredLogger
}

func NewProjectManager(fs fs.FS, entryBuilder FileEntryBuilder, logger *zap.SugaredLogger) ProjectManager {
	return ProjectManager{fs: fs, entryBuilder: entryBuilder, logger: logger}
}

// Load uses a given path to a project and loads it into a [Project].
func (p ProjectManager) Load(projectPath string) (*Project, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	mainDeclarativeComponents := make([]MainDeclarativeComponent, 0, len(projectMainComponentPaths))
	for _, mainComponentPath := range projectMainComponentPaths {
		fullMainComponentPath := projectPath + mainComponentPath
		p.logger.Debugf("walking main component path %s", fullMainComponentPath)
		entryFilePath := fullMainComponentPath + "/" + api.EntryFileName
		if _, err := fs.Stat(p.fs, entryFilePath); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: could not load %s", ErrMainComponentNotFound, entryFilePath)
		}
		mainDeclarativeComponentEntry, err := p.entryBuilder.Build(entryFilePath)
		if err != nil {
			return nil, err
		}
		p.logger.Infof("found main declarative component %s", fullMainComponentPath)

		mainDeclarativeSubComponents := make([]*SubDeclarativeComponent, 0, 10)
		subComponentsByPath := make(map[string]*SubDeclarativeComponent)
		err = fs.WalkDir(p.fs, fullMainComponentPath, func(path string, dirEntry fs.DirEntry, err error) error {
			p.logger.Debugf("walking path %s", path)
			parentPath := strings.TrimSuffix(path, "/"+dirEntry.Name())
			if !dirEntry.IsDir() && parentPath == fullMainComponentPath {
				if dirEntry.Name() == api.EntryFileName {
					p.logger.Debugf("skipping entry %s as it is part of a main declarative component", path)
				} else {
					p.logger.Debugf("skipping file %s as it is not part of a sub declarative component", path)
				}
				return nil
			}

			if dirEntry.IsDir() && path == fullMainComponentPath {
				p.logger.Debugf("skipping directory %s as it is a main component", path)
				return nil
			}

			parent := subComponentsByPath[parentPath]
			if dirEntry.IsDir() {
				entryFilePath = path + "/" + api.EntryFileName
				if _, err := fs.Stat(p.fs, entryFilePath); errors.Is(err, fs.ErrNotExist) {
					p.logger.Infof("skipping directory %s, because no entry.cue was found", dirEntry.Name())
					return filepath.SkipDir
				}
				entry, err := p.entryBuilder.Build(entryFilePath)
				if err != nil {
					return err
				}
				p.logger.Infof("found sub declarative component %s", path)
				subDeclarativeComponent := &SubDeclarativeComponent{entry: *entry}
				subComponentsByPath[path] = subDeclarativeComponent
				if parentPath != fullMainComponentPath {
					parent.subComponents = append(parent.subComponents, subDeclarativeComponent)
				} else {
					mainDeclarativeSubComponents = append(mainDeclarativeSubComponents, subDeclarativeComponent)
				}
			} else if dirEntry.Name() != api.EntryFileName {
				p.logger.Infof("found sub declarative component manifest %s", dirEntry.Name())
				p.logger.Debugf("adding sub declarative component manifest %s to parent sub declarative component %s", dirEntry.Name(), parentPath)
				parent.manifests = append(parent.manifests, &Manifest{name: dirEntry.Name()})
			} else {
				p.logger.Debugf("skipping entry %s as it is already included", path)
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadProject, err)
		}

		mainDeclarativeComponents = append(mainDeclarativeComponents, NewMainDeclarativeComponent(*mainDeclarativeComponentEntry, mainDeclarativeSubComponents))
	}

	proj := NewProject(mainDeclarativeComponents)
	return &proj, nil
}
