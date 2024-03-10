package build

import (
	"context"
	"os"
)

type WorkflowsGen struct {
	Export bool
}

var _ step = (*WorkflowsGen)(nil)

func (_ WorkflowsGen) name() string {
	return "Generate Workflows"
}

// when changed, the renovate customManager has also to be updated.
var cueDep = "cuelang.org/go/cmd/cue@v0.8.0"

func (workflow WorkflowsGen) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	workflowsDir := ".github/workflows"
	gen := request.container.
		WithExec([]string{"mkdir", "-p", workflowsDir}).
		WithExec([]string{"go", "install", cueDep}).
		WithEnvVariable("CUE_EXPERIMENT", "modules").
		WithEnvVariable("CUE_REGISTRY", "ghcr.io/kharf").
		WithWorkdir("build").
		WithExec([]string{"../bin/cue", "cmd", "genyamlworkflows"}).
		WithWorkdir(workDir)

	if workflow.Export {
		_, err := gen.Directory(workflowsDir).Export(ctx, workflowsDir)
		if err != nil {
			return nil, err
		}
	}

	return &stepResult{
		container: gen,
	}, nil
}

type commitWorkflows struct{}

var CommitWorkflows = commitWorkflows{}

var _ step = (*commitWorkflows)(nil)

func (_ commitWorkflows) name() string {
	return "Commit Workflows"
}

func (_ commitWorkflows) run(ctx context.Context, request stepRequest) (*stepResult, error) {
	pat := request.client.SetSecret("gh-token", os.Getenv("GH_PAT"))

	commitContainer, err := request.container.
		WithExec([]string{"sh", "-c", "git diff --exit-code .github; echo -n $? > /exit_code"}).
		Sync(ctx)
	if err != nil {
		return nil, err
	}

	exitCode, err := commitContainer.File("/exit_code").Contents(ctx)
	if err != nil {
		return nil, err
	}

	lastContainer := commitContainer
	if exitCode != "0" {
		lastContainer = commitContainer.
			WithSecretVariable("GH_PAT", pat).
			WithExec([]string{"git", "config", "--global", "user.email", "bot@declcd.io"}).
			WithExec([]string{"git", "config", "--global", "user.name", "Declcd Bot"}).
			WithExec([]string{"sh", "-c", "git remote set-url origin https://$GH_PAT@github.com/kharf/declcd.git"}).
			WithExec([]string{"git", "add", ".github/workflows"}).
			WithExec([]string{"git", "commit", "-m", "chore: update yaml workflows"}).
			WithExec([]string{"git", "push"})
	}

	return &stepResult{
		container: lastContainer,
	}, nil
}
