package main

import (
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue/cuecontext"
	"github.com/kharf/declcd/pkg/core"
	"go.uber.org/zap"
)

// WIP
func main() {
	basicLogger, err := initZap()
	if err != nil {
		panic(err)
	}
	logger := basicLogger.Sugar()

	repositoryManager := core.NewRepositoryManager()
	rootDir := "/tmp"
	repositoryDir := "decl"
	localRepositoryPath := filepath.Join(rootDir, repositoryDir)
	_, err = repositoryManager.Clone(core.WithUrl("https://github.com/kharf/declcd-test-repo.git"), core.WithTarget(localRepositoryPath))
	if err != nil {
		panic(err)
	}

	fileSystem := os.DirFS(rootDir)
	ctx := cuecontext.New()
	builder := core.NewFileEntryBuilder(ctx, fileSystem, core.NewContentEntryBuilder(ctx))
	projectManager := core.NewProjectManager(fileSystem, builder, logger)
	project, err := projectManager.Load(repositoryDir)
	if err != nil {
		panic(err)
	}

	manifestBuilder := core.NewComponentManifestBuilder(ctx)
	for _, component := range project.MainComponents {
		buildSubComponent(localRepositoryPath, manifestBuilder, component.SubComponents)
	}
}

func buildSubComponent(localRepositoryPath string, builder core.ComponentManifestBuilder, components []*core.SubDeclarativeComponent) {
	for _, component := range components {
		unstructureds, err := builder.Build(core.WithProjectRoot(localRepositoryPath), core.WithComponent(component.Entry.Name, component.Path))
		if err != nil {
			panic(err)
		}

		for _, unstructured := range unstructureds {
			fmt.Println("built manifest", unstructured.Object)
		}
	}
}

func initZap() (*zap.Logger, error) {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.OutputPaths = []string{"stdout"}
	return zapConfig.Build()
}
