package container_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/helmtest"
	"github.com/kharf/declcd/internal/kubetest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/projecttest"
	"github.com/kharf/declcd/pkg/container"
	"gotest.tools/v3/assert"
)

func TestUpdate(t *testing.T) {
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	tlsRegistry, err := ocitest.NewTLSRegistry(false, "")
	assert.NilError(t, err)
	defer tlsRegistry.Close()

	registryPath, err := os.MkdirTemp("", "declcd-cue-registry*")
	assert.NilError(t, err)
	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assert.NilError(t, err)
	defer cueModuleRegistry.Close()

	remote.DefaultTransport = tlsRegistry.Client().Transport

	publicHelmEnvironment, err := helmtest.NewHelmEnvironment(
		helmtest.WithOCI(false),
		helmtest.WithPrivate(false),
	)
	assert.NilError(t, err)
	defer publicHelmEnvironment.Close()

	testCases := []struct {
		name            string
		repository      map[string][]string
		container1      string
		container2      string
		expectedUpdates map[int]string
		expectedErr     string
	}{
		{
			name:       "No-Updates",
			container1: "one:1.14.2",
			container2: "two:1.15.0",
			repository: map[string][]string{
				"one": {"1.1.1"},
				"two": {"1.14.1", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			},
		},
		{
			name:       "Updates",
			container1: "one:1.14.2",
			container2: "two:1.14.2",
			repository: map[string][]string{
				"one": {"1.1.1"},
				"two": {"1.14.1", "notsemver", "1.2.6", "1.15.0", "1.2", "1.2.2"},
			},
			expectedUpdates: map[int]string{
				37: "two:1.15.0",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := projecttest.StartProjectEnv(t,
				projecttest.WithProjectSource("simple"),
				projecttest.WithKubernetes(
					kubetest.WithEnabled(false),
				),
			)
			defer env.Stop()

			deploymentRelativeFilePath := "infra/prometheus/subcomponent/deployment.cue"

			err = helmtest.ReplaceTemplate(
				helmtest.Template{
					Name:                    "test",
					TestProjectPath:         env.Projects[0].TargetPath,
					RelativeReleaseFilePath: "infra/prometheus/releases.cue",
					RepoURL:                 publicHelmEnvironment.ChartServer.URL(),
				},
				env.Projects[0].GitRepository,
			)
			assert.NilError(t, err)
			err = projecttest.ReplaceTemplate(
				projecttest.Template{
					TestProjectPath:  env.Projects[0].TargetPath,
					RelativeFilePath: deploymentRelativeFilePath,
					Data: struct {
						Container1 string
						Container2 string
					}{
						Container1: fmt.Sprintf("%v/%v", tlsRegistry.Addr(), tc.container1),
						Container2: fmt.Sprintf("%v/%v", tlsRegistry.Addr(), tc.container2),
					},
				},
				env.Projects[0].GitRepository,
			)
			assert.NilError(t, err)

			dag, err := env.ProjectManager.Load(env.Projects[0].TargetPath)
			assert.NilError(t, err)

			components, err := dag.TopologicalSort()
			assert.NilError(t, err)

			ctx := context.Background()
			for repo, tags := range tc.repository {
				for _, tag := range tags {
					desc, err := tlsRegistry.PushManifest(
						ctx,
						repo,
						tag,
						[]byte{},
						"application/vnd.docker.distribution.manifest.v2+json",
					)
					assert.NilError(t, err)
					defer tlsRegistry.DeleteManifest(ctx, repo, desc.Digest)
				}
			}

			repo := env.Projects[0].GitRepository.Repository
			updater := &container.Updater{
				GitRepository: repo,
			}

			updateResult, err := updater.Update(components)
			if tc.expectedErr == "" {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, tc.expectedErr)
			}

			assert.Assert(t, len(updateResult.Updates) == len(tc.expectedUpdates))

			deploymentFile, err := os.Open(
				filepath.Join(env.Projects[0].TargetPath, deploymentRelativeFilePath),
			)
			assert.NilError(t, err)

			for _, update := range updateResult.Updates {
				commit, err := repo.CommitObject(plumbing.NewHash(update.CommitHash))
				assert.NilError(t, err)

				_, err = commit.File(deploymentRelativeFilePath)
				assert.NilError(t, err)

				deploymentFileContent, err := io.ReadAll(deploymentFile)
				assert.NilError(t, err)
				lines := strings.Split(string(deploymentFileContent), "\n")

				assert.Assert(t, len(lines) >= update.Line)
				assert.Equal(
					t,
					fmt.Sprintf("%s:%s", update.Image, update.NewVersion),
					fmt.Sprintf("%s/%s", tlsRegistry.Addr(), tc.expectedUpdates[update.Line]),
				)
			}
		})
	}
}
