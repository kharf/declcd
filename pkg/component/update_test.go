// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/kharf/declcd/internal/dnstest"
	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/internal/txtar"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/project"
	"github.com/kharf/declcd/pkg/vcs"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
)

func TestUpdater_Update(t *testing.T) {
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	tlsRegistry, err := ocitest.NewTLSRegistry(false, "")
	assert.NilError(t, err)
	defer tlsRegistry.Close()

	registryPath := t.TempDir()
	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assert.NilError(t, err)
	defer cueModuleRegistry.Close()

	const manifestProjectTemplate = `
-- cue.mod/module.cue --
module: "container.update@v0"
language: version: "v0.8.0"
-- apps/app.cue --
package apps

deployment: {
	type: "Manifest"
	id: ""
	dependencies: []
	content: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      "deployment"
		}
		spec: {
			template: {
				spec: {
					containers: [
						{
							name:  "container"
							image: "{{.Container}}:{{.ContainerVersion}}" @update(strategy=semver, constraint="{{.Constraint}}", secret=registry)
							ports: [{
								containerPort: 80
							}]
						},
					]
				}
			}
		}
	}
}
`

	testCases := []struct {
		name             string
		container        string
		containerVersion string
		constraint       string
		registryVersions []string
		wantVersion      string
		wantErr          string
	}{
		{
			name:             "No-Update",
			containerVersion: "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:      "1.14.2",
		},
		{
			name:             "No-Matching-Constraint",
			containerVersion: "1.14.2",
			constraint:       "< 1.1.3",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:      "1.14.2",
		},
		{
			name:             "Invalid-Semver-Version",
			containerVersion: "latest",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantErr:          "Invalid Semantic Version",
		},
		{
			name:             "No-Remote-Semver-Version",
			containerVersion: "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"notsemver"},
			wantVersion:      "1.14.2",
		},
		{
			name:             "Container-Not-Found",
			container:        "idontexist",
			containerVersion: "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"notsemver"},
			wantErr:          "repository name not known to registry",
		},
		{
			name:             "Update",
			containerVersion: "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{
				"1.14.3",
				"1.15.0",
				"notsemver",
				"1.2.6",
				"1.3",
				"3.6.4",
				"2.0.0",
				"0.8.0",
				"4.6.9",
				"0.8.5",
				"1.2",
				"1.2.2",
			},
			wantVersion: "1.15.0",
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, tag := range tc.registryVersions {
				desc, err := tlsRegistry.PushManifest(
					ctx,
					"container",
					tag,
					[]byte{},
					"application/vnd.docker.distribution.manifest.v2+json",
				)
				assert.NilError(t, err)
				defer tlsRegistry.DeleteManifest(ctx, "container", desc.Digest)
			}

			parsedTemplate, err := template.New("").Parse(manifestProjectTemplate)
			assert.NilError(t, err)
			buf := &bytes.Buffer{}
			containerName := "container"
			if tc.container != "" {
				containerName = tc.container
			}
			containerImage := fmt.Sprintf("%v/%v", tlsRegistry.Addr(), containerName)
			err = parsedTemplate.Execute(buf, struct {
				Container        string
				ContainerVersion string
				Constraint       string
			}{
				Container:        containerImage,
				ContainerVersion: tc.containerVersion,
				Constraint:       tc.constraint,
			})
			assert.NilError(t, err)
			projectDir := t.TempDir()
			err = txtar.Create(projectDir, buf)
			assert.NilError(t, err)

			pm := project.NewManager(component.NewBuilder(), runtime.GOMAXPROCS(0))
			projectInstance, err := pm.Load(projectDir)
			assert.NilError(t, err)
			assert.Assert(t, projectInstance != nil)

			repo, err := gittest.InitGitRepository(t, t.TempDir(), projectDir, "main")
			assert.NilError(t, err)

			vcsRepository, err := vcs.Open("main", projectDir, nil)
			assert.NilError(t, err)

			updater := &component.Updater{
				Repository: vcsRepository,
			}

			updates, err := updater.Update(projectInstance.UpdateInstructions)
			if tc.wantErr == "" {
				assert.NilError(t, err)

				deploymentRelativeFilePath := filepath.Join("apps", "app.cue")
				deploymentFile, err := os.Open(
					filepath.Join(projectDir, deploymentRelativeFilePath),
				)
				assert.NilError(t, err)

				deploymentFileContent, err := io.ReadAll(deploymentFile)
				assert.NilError(t, err)
				lines := strings.Split(string(deploymentFileContent), "\n")

				containerLine := lines[18]

				assert.Equal(
					t,
					strings.TrimSpace(containerLine),
					fmt.Sprintf(
						"image: \"%s:%s\" @update(strategy=semver, constraint=\"%s\", secret=registry)",
						containerImage,
						tc.wantVersion,
						tc.constraint,
					),
				)

				if tc.containerVersion == tc.wantVersion {
					assert.Assert(t, len(updates) == 0)
					return
				}

				assert.Assert(t, len(updates) == 1)

				update := updates[0]
				assert.Equal(t, update.NewVersion, tc.wantVersion)

				commit, err := repo.Repository.CommitObject(plumbing.NewHash(update.CommitHash))
				assert.NilError(t, err)

				assert.Equal(t, len(projectInstance.UpdateInstructions), 1)
				assert.Equal(
					t,
					projectInstance.UpdateInstructions[0].Target.GetStructValue(),
					fmt.Sprintf("%s:%s", containerImage, update.NewVersion),
				)

				_, err = commit.File(deploymentRelativeFilePath)
				assert.NilError(t, err)
				assert.Equal(t, commit.Author.Name, "declcd-bot")
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestUpdater_Update_HelmOCI(t *testing.T) {
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	tlsRegistry, err := ocitest.NewTLSRegistry(false, "")
	assert.NilError(t, err)
	defer tlsRegistry.Close()

	registryPath := t.TempDir()
	cueModuleRegistry, err := ocitest.StartCUERegistry(registryPath)
	assert.NilError(t, err)
	defer cueModuleRegistry.Close()

	const helmProjectTemplate = `
-- cue.mod/module.cue --
module: "helm.update@v0"
language: version: "v0.8.0"

-- apps/app.cue --
package apps

release: {
	type: "HelmRelease"
	id: ""
	dependencies: []
	name: "release"
	namespace: "default"
	chart: {
		name: "{{.Name}}"
		repoURL: "{{.Repo}}"
		version: "{{.Version}}"
	} @update(constraint="{{.Constraint}}")
	values: {}
	patches: []
	crds: allowUpgrade: false
}
`

	testCases := []struct {
		name             string
		chartName        string
		version          string
		constraint       string
		registryVersions []string
		wantVersion      string
		wantErr          string
	}{
		{
			name:             "No-Update",
			version:          "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:      "1.14.2",
		},
		{
			name:             "No-Matching-Constraint",
			version:          "1.14.2",
			constraint:       "< 1.1.3",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:      "1.14.2",
		},
		{
			name:             "Wrong-Chart-Name-Format",
			chartName:        ":://wrong",
			version:          "1.14.2",
			constraint:       "< 1.1.3",
			registryVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantErr:          "repository can only contain the characters `abcdefghijklmnopqrstuvwxyz0123456789_-./`: :://wrong",
		},
		{
			name:             "Container-Not-Found",
			chartName:        "idontexist",
			version:          "1.14.2",
			constraint:       "<= 1.15.3, >= 1.4",
			registryVersions: []string{"notsemver"},
			wantErr:          "repository name not known to registry",
		},
		{
			name:       "Update",
			version:    "1.14.2",
			constraint: "<= 1.15.3, >= 1.4",
			registryVersions: []string{
				"1.14.3",
				"1.15.0",
				"notsemver",
				"1.2.6",
				"1.3",
				"3.6.4",
				"2.0.0",
				"0.8.0",
				"4.6.9",
				"0.8.5",
				"1.2",
				"1.2.2",
			},
			wantVersion: "1.15.0",
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, tag := range tc.registryVersions {
				desc, err := tlsRegistry.PushManifest(
					ctx,
					"container",
					tag,
					[]byte{},
					"application/vnd.cncf.helm.config.v1+json",
				)
				assert.NilError(t, err)
				defer tlsRegistry.DeleteManifest(ctx, "container", desc.Digest)
			}

			parsedTemplate, err := template.New("").Parse(helmProjectTemplate)
			assert.NilError(t, err)

			buf := &bytes.Buffer{}
			chartName := "container"
			if tc.chartName != "" {
				chartName = tc.chartName
			}
			err = parsedTemplate.Execute(buf, struct {
				Repo       string
				Version    string
				Constraint string
				Name       string
			}{
				Repo:       tlsRegistry.URL(),
				Version:    tc.version,
				Constraint: tc.constraint,
				Name:       chartName,
			})
			assert.NilError(t, err)
			projectDir := t.TempDir()
			err = txtar.Create(projectDir, buf)
			assert.NilError(t, err)

			pm := project.NewManager(component.NewBuilder(), runtime.GOMAXPROCS(0))
			projectInstance, err := pm.Load(projectDir)
			assert.NilError(t, err)
			assert.Assert(t, projectInstance != nil)

			repo, err := gittest.InitGitRepository(t, t.TempDir(), projectDir, "main")
			assert.NilError(t, err)

			vcsRepository, err := vcs.Open("main", projectDir, nil)
			assert.NilError(t, err)

			updater := &component.Updater{
				Repository: vcsRepository,
			}

			updates, err := updater.Update(projectInstance.UpdateInstructions)
			if tc.wantErr == "" {
				assert.NilError(t, err)

				appRelativeFilePath := filepath.Join("apps", "app.cue")
				file, err := os.Open(
					filepath.Join(projectDir, appRelativeFilePath),
				)
				assert.NilError(t, err)

				fileContent, err := io.ReadAll(file)
				assert.NilError(t, err)
				lines := strings.Split(string(fileContent), "\n")

				containerLine := lines[11]

				assert.Equal(
					t,
					strings.TrimSpace(containerLine),
					fmt.Sprintf(
						"version: \"%s\"",
						tc.wantVersion,
					),
				)

				if tc.version == tc.wantVersion {
					assert.Assert(t, len(updates) == 0)
					return
				}

				assert.Assert(t, len(updates) == 1)

				update := updates[0]
				assert.Equal(t, update.NewVersion, tc.wantVersion)

				commit, err := repo.Repository.CommitObject(plumbing.NewHash(update.CommitHash))
				assert.NilError(t, err)

				assert.Equal(t, len(projectInstance.UpdateInstructions), 1)
				assert.Equal(
					t,
					projectInstance.UpdateInstructions[0].Target.GetStructValue(),
					update.NewVersion,
				)

				_, err = commit.File(appRelativeFilePath)
				assert.NilError(t, err)
				assert.Equal(t, commit.Author.Name, "declcd-bot")
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestUpdater_Update_HelmHTTP(t *testing.T) {
	dnsServer, err := dnstest.NewDNSServer()
	assert.NilError(t, err)
	defer dnsServer.Close()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	http.DefaultTransport = transport

	const helmProjectTemplate = `
-- cue.mod/module.cue --
module: "helm.update@v0"
language: version: "v0.8.0"

-- apps/app.cue --
package apps

release: {
	type: "HelmRelease"
	id: ""
	dependencies: []
	name: "release"
	namespace: "default"
	chart: {
		name: "{{.Name}}"
		repoURL: "{{.Repo}}"
		version: "{{.Version}}"
	} @update(constraint="{{.Constraint}}")
	values: {}
	patches: []
	crds: allowUpgrade: false
}
`

	testCases := []struct {
		name          string
		chartName     string
		version       string
		constraint    string
		chartVersions []string
		wantVersion   string
		wantErr       string
	}{
		{
			name:          "No-Update",
			version:       "1.14.2",
			constraint:    "<= 1.15.3, >= 1.4",
			chartVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:   "1.14.2",
		},
		{
			name:          "No-Matching-Constraint",
			version:       "1.14.2",
			constraint:    "< 1.1.3",
			chartVersions: []string{"1.14.2", "notsemver", "1.2.6", "1.13.0", "1.2", "1.2.2"},
			wantVersion:   "1.14.2",
		},
		{
			name:          "Chart-Not-Found",
			chartName:     "idontexist",
			version:       "1.14.2",
			constraint:    "<= 1.15.3, >= 1.4",
			chartVersions: []string{"notsemver"},
			wantErr:       "Chart not found",
		},
		{
			name:       "Update",
			version:    "1.14.2",
			constraint: "<= 1.15.3, >= 1.4",
			chartVersions: []string{
				"1.14.3",
				"1.15.0",
				"notsemver",
				"1.2.6",
				"1.3",
				"3.6.4",
				"2.0.0",
				"0.8.0",
				"4.6.9",
				"0.8.5",
				"1.2",
				"1.2.2",
			},
			wantVersion: "1.15.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			versions := make(repo.ChartVersions, 0, len(tc.chartVersions))
			for _, version := range tc.chartVersions {
				versions = append(versions, &repo.ChartVersion{
					Metadata: &chart.Metadata{
						Version: version,
					},
				})
			}
			chartName := "chart"
			if tc.chartName != "" {
				chartName = tc.chartName
			}
			indexFile := &repo.IndexFile{
				Entries: map[string]repo.ChartVersions{
					"chart": versions,
				},
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
				encoder := yaml.NewEncoder(w)
				if err := encoder.Encode(indexFile); err != nil {
					w.WriteHeader(500)
					w.Write([]byte(err.Error()))
				}
			})
			helmServer := httptest.NewTLSServer(mux)
			defer helmServer.Close()

			parsedTemplate, err := template.New("").Parse(helmProjectTemplate)
			assert.NilError(t, err)

			buf := &bytes.Buffer{}
			err = parsedTemplate.Execute(buf, struct {
				Repo       string
				Version    string
				Constraint string
				Name       string
			}{
				Repo:       helmServer.URL,
				Version:    tc.version,
				Constraint: tc.constraint,
				Name:       chartName,
			})
			assert.NilError(t, err)
			projectDir := t.TempDir()
			err = txtar.Create(projectDir, buf)
			assert.NilError(t, err)

			pm := project.NewManager(component.NewBuilder(), runtime.GOMAXPROCS(0))
			projectInstance, err := pm.Load(projectDir)
			assert.NilError(t, err)
			assert.Assert(t, projectInstance != nil)

			repo, err := gittest.InitGitRepository(t, t.TempDir(), projectDir, "main")
			assert.NilError(t, err)

			vcsRepository, err := vcs.Open("main", projectDir, nil)
			assert.NilError(t, err)

			updater := &component.Updater{
				Repository: vcsRepository,
			}

			updates, err := updater.Update(projectInstance.UpdateInstructions)
			if tc.wantErr == "" {
				assert.NilError(t, err)

				appRelativeFilePath := filepath.Join("apps", "app.cue")
				file, err := os.Open(
					filepath.Join(projectDir, appRelativeFilePath),
				)
				assert.NilError(t, err)

				fileContent, err := io.ReadAll(file)
				assert.NilError(t, err)
				lines := strings.Split(string(fileContent), "\n")

				containerLine := lines[11]

				assert.Equal(
					t,
					strings.TrimSpace(containerLine),
					fmt.Sprintf(
						"version: \"%s\"",
						tc.wantVersion,
					),
				)

				if tc.version == tc.wantVersion {
					assert.Assert(t, len(updates) == 0)
					return
				}

				assert.Assert(t, len(updates) == 1)

				update := updates[0]
				assert.Equal(t, update.NewVersion, tc.wantVersion)

				commit, err := repo.Repository.CommitObject(plumbing.NewHash(update.CommitHash))
				assert.NilError(t, err)

				assert.Equal(t, len(projectInstance.UpdateInstructions), 1)
				assert.Equal(
					t,
					projectInstance.UpdateInstructions[0].Target.GetStructValue(),
					update.NewVersion,
				)

				_, err = commit.File(appRelativeFilePath)
				assert.NilError(t, err)
				assert.Equal(t, commit.Author.Name, "declcd-bot")
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
