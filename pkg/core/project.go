package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/go-git/go-git/v5"
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

// Project is the declcd representation of the "GitOps" repository with all its declarative components.
type Project struct {
	MainComponents []MainDeclarativeComponent
}

func NewProject(mainComponents []MainDeclarativeComponent) Project {
	return Project{MainComponents: mainComponents}
}

// ProjectManager loads a declcd [Project] from given File System.
type ProjectManager struct {
	fs     fs.FS
	logger *zap.SugaredLogger
}

func NewProjectManager(fs fs.FS, logger *zap.SugaredLogger) ProjectManager {
	return ProjectManager{fs: fs, logger: logger}
}

// Load uses a given path to a project and loads it into a [Project].
func (p ProjectManager) Load(projectPath string) (*Project, error) {
	projectPath = strings.TrimSuffix(projectPath, "/")
	mainDeclarativeComponents := make([]MainDeclarativeComponent, 0, len(projectMainComponentPaths))
	for _, mainComponentPath := range projectMainComponentPaths {
		fullMainComponentPath := projectPath + mainComponentPath
		p.logger.Debugf("walking main component path %s", fullMainComponentPath)
		if _, err := fs.Stat(p.fs, fullMainComponentPath); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: could not load %s", ErrMainComponentNotFound, fullMainComponentPath)
		}
		p.logger.Infof("found main declarative component %s", fullMainComponentPath)

		mainDeclarativeSubComponents := make([]*SubDeclarativeComponent, 0, 10)
		subComponentsByPath := make(map[string]*SubDeclarativeComponent)
		err := fs.WalkDir(p.fs, fullMainComponentPath, func(path string, dirEntry fs.DirEntry, err error) error {
			p.logger.Debugf("walking path %s", path)
			parentPath := strings.TrimSuffix(path, "/"+dirEntry.Name())
			if !dirEntry.IsDir() && parentPath == fullMainComponentPath {
				if dirEntry.Name() == ComponentFileName {
					p.logger.Debugf("skipping component %s as it is part of a main declarative component", path)
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
				componentFilePath := path + "/" + ComponentFileName
				if _, err := fs.Stat(p.fs, componentFilePath); errors.Is(err, fs.ErrNotExist) {
					p.logger.Infof("skipping directory %s, because no component.cue was found", dirEntry.Name())
					return filepath.SkipDir
				}
				p.logger.Infof("found sub declarative component %s", path)
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

	proj := NewProject(mainDeclarativeComponents)
	return &proj, nil
}

// A vcs Repository.
type Repository struct {
	path string
}

func NewRepository(path string) Repository {
	return Repository{path: path}
}

// RepositoryManager clones a remote vcs repository to a local path.
type RepositoryManager struct{}

func NewRepositoryManager() RepositoryManager {
	return RepositoryManager{}
}

// CloneOptions define configuration how to clone a vcs repository.
type CloneOptions struct {
	// Location of the remote vcs repository.
	url string
	// Location to where the vcs repository is loaded.
	targetPath string
}

type cloneOption = func(opt *CloneOptions)

// WithUrl provides a URL configuration for the clone function.
func WithUrl(url string) func(*CloneOptions) {
	return func(opt *CloneOptions) {
		opt.url = url
	}
}

// WithTarget provides a local path to where the vcs repository is cloned.
func WithTarget(path string) func(*CloneOptions) {
	return func(opt *CloneOptions) {
		opt.targetPath = path
	}
}

// Clone loads a remote vcs repository to a local path.
func (manager RepositoryManager) Clone(opts ...cloneOption) (*Repository, error) {
	options := &CloneOptions{}
	for _, opt := range opts {
		opt(options)
	}

	targetPath := options.targetPath
	if _, err := git.PlainClone(
		targetPath, false,
		&git.CloneOptions{URL: options.url, Progress: os.Stdout},
	); err != nil && err != git.ErrRepositoryAlreadyExists {
		return nil, err
	}

	repository := NewRepository(targetPath)
	return &repository, nil
}
