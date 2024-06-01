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

package helmtest

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	helmKube "helm.sh/helm/v3/pkg/kube"
	helmRegistry "helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

func ConfigureHelm(cfg *rest.Config) (*action.Configuration, error) {
	helmCfg := action.Configuration{}
	helmKube.ManagedFieldsManager = project.ControllerName

	k8sClient, err := kube.NewDynamicClient(cfg)
	if err != nil {
		return nil, err
	}

	getter := &kube.InMemoryRESTClientGetter{
		Cfg:        cfg,
		RestMapper: k8sClient.RESTMapper(),
	}

	err = helmCfg.Init(getter, "default", "secret", log.Printf)
	if err != nil {
		return nil, err
	}

	helmCfg.KubeClient = &kube.HelmClient{
		Client:        helmCfg.KubeClient.(*helmKube.Client),
		DynamicClient: k8sClient,
		FieldManager:  "controller",
	}

	return &helmCfg, nil
}

type projectOption struct {
	repo        *gittest.LocalGitRepository
	testProject string
	testRoot    string
}

var _ Option = (*projectOption)(nil)

func (opt projectOption) Apply(opts *options) {
	opts.project = opt
}

type enabled bool

var _ Option = (*enabled)(nil)

func (opt enabled) Apply(opts *options) {
	opts.enabled = bool(opt)
}

type oci bool

var _ Option = (*oci)(nil)

func (opt oci) Apply(opts *options) {
	opts.oci = bool(opt)
}

type private bool

var _ Option = (*private)(nil)

func (opt private) Apply(opts *options) {
	opts.private = bool(opt)
}

type provider cloud.ProviderID

var _ Option = (*provider)(nil)

func (opt provider) Apply(opts *options) {
	opts.cloudProviderID = cloud.ProviderID(opt)
}

type options struct {
	enabled         bool
	oci             bool
	private         bool
	project         projectOption
	cloudProviderID cloud.ProviderID
}

type Option interface {
	Apply(*options)
}

func Enabled(isEnabled bool) enabled {
	return enabled(isEnabled)
}

func WithOCI(enabled bool) oci {
	return oci(enabled)
}

func WithPrivate(enabled bool) private {
	return private(enabled)
}

func WithProject(
	repo *gittest.LocalGitRepository,
	testProject string,
	testRoot string,
) projectOption {
	return projectOption{
		repo:        repo,
		testProject: testProject,
		testRoot:    testRoot,
	}
}

func WithProvider(providerID cloud.ProviderID) provider {
	return provider(providerID)
}

type Server interface {
	// base URL of form http://ipaddr:port with no trailing slash
	URL() string
	Addr() string
	Close()
}

type ociRegistry struct {
	server *ocitest.Registry
}

var _ Server = (*ociRegistry)(nil)

func (r *ociRegistry) Close() {
	r.server.Close()
}

func (r *ociRegistry) URL() string {
	return r.server.URL()
}

func (r *ociRegistry) Addr() string {
	return r.server.Addr()
}

type yamlBasedRepository struct {
	server *httptest.Server
}

var _ Server = (*yamlBasedRepository)(nil)

func (r *yamlBasedRepository) Close() {
	r.server.Close()
}

func (r *yamlBasedRepository) URL() string {
	return r.server.URL
}

func (r *yamlBasedRepository) Addr() string {
	return r.server.Config.Addr
}

type Environment struct {
	ChartServer   Server
	chartArchives []*os.File
}

func (env Environment) Close() {
	if env.ChartServer != nil {
		env.ChartServer.Close()
	}
	for _, f := range env.chartArchives {
		os.Remove(f.Name())
	}
}

// NewHelmEnvironment creates Helm chart archives and starts either and oci or yaml based Helm repository.
func NewHelmEnvironment(opts ...Option) (*Environment, error) {
	options := &options{
		enabled:         false,
		private:         false,
		oci:             false,
		cloudProviderID: "",
	}
	for _, o := range opts {
		o.Apply(options)
	}

	v1Archive, err := createChartArchive("test", "1.0.0")
	if err != nil {
		return nil, err
	}

	v2Archive, err := createChartArchive("testv2", "2.0.0")
	if err != nil {
		return nil, err
	}

	var chartServer Server
	if options.oci {
		var err error
		ociServer, err := ocitest.NewTLSRegistry(
			options.private,
			string(options.cloudProviderID),
		)
		if err != nil {
			return nil, err
		}

		helmOpts := []helmRegistry.ClientOption{
			helmRegistry.ClientOptDebug(true),
			helmRegistry.ClientOptWriter(os.Stderr),
			helmRegistry.ClientOptHTTPClient(ociServer.Client()),
			helmRegistry.ClientOptResolver(nil),
		}

		helmRegistryClient, err := helmRegistry.NewClient(helmOpts...)
		if err != nil {
			return nil, err
		}

		v1Bytes, err := os.ReadFile(v1Archive.Name())
		if err != nil {
			return nil, err
		}

		_, err = helmRegistryClient.Push(
			v1Bytes,
			fmt.Sprintf("%s/%s:%s", ociServer.Addr(), "test", "1.0.0"),
		)
		if err != nil {
			return nil, err
		}

		chartServer = &ociRegistry{
			server: ociServer,
		}
	} else {
		httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if options.private {
				auth, found := r.Header["Authorization"]
				if !found {
					w.WriteHeader(500)
					return
				}

				if len(auth) != 1 {
					w.WriteHeader(500)
					return
				}

				// declcd:abcd
				if auth[0] != "Basic ZGVjbGNkOmFiY2Q=" {
					w.WriteHeader(500)
					return

				}
			}

			if strings.HasSuffix(r.URL.Path, "index.yaml") {
				index := &repo.IndexFile{
					APIVersion: "v1",
					Generated:  time.Now(),
					Entries: map[string]repo.ChartVersions{
						"test": {
							&repo.ChartVersion{
								Metadata: &chart.Metadata{
									APIVersion: "v1",
									Version:    "1.0.0",
									Name:       "test",
								},
								URLs: []string{chartServer.URL() + "/test-1.0.0.tgz"},
							},
							&repo.ChartVersion{
								Metadata: &chart.Metadata{
									APIVersion: "v1",
									Version:    "2.0.0",
									Name:       "test",
								},
								URLs: []string{chartServer.URL() + "/test-2.0.0.tgz"},
							},
						},
					},
				}

				indexBytes, err := yaml.Marshal(index)
				if err != nil {
					w.WriteHeader(500)
					return
				}

				if _, err := w.Write(indexBytes); err != nil {
					w.WriteHeader(500)
					return
				}
				return
			}
			archive := v1Archive
			if strings.Contains(r.URL.Path, "2.0.0") {
				archive = v2Archive
			}

			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(archive.Name())))

			file, err := os.Open(archive.Name())
			if err != nil {
				w.WriteHeader(500)
				return
			}

			if _, err := io.Copy(w, file); err != nil {
				w.WriteHeader(500)
				return
			}
		}))

		chartServer = &yamlBasedRepository{
			server: httpsServer,
		}
	}

	return &Environment{
		ChartServer:   chartServer,
		chartArchives: []*os.File{v1Archive, v2Archive},
	}, nil
}

func createChartArchive(chart string, version string) (*os.File, error) {
	archive, err := os.CreateTemp("", fmt.Sprintf("*-test-%s.tgz", version))
	if err != nil {
		return nil, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	gzWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzWriter)
	chartDir := filepath.Join(dir, "test", "testdata", "charts")
	walkDirErr := fs.WalkDir(
		os.DirFS(chartDir),
		chart,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() || path == ".helmignore" {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			header := &tar.Header{
				Name: path,
				Mode: int64(info.Mode()),
				Size: info.Size(),
			}

			if err := tarWriter.WriteHeader(header); err != nil {
				return err
			}

			file, err := os.Open(filepath.Join(chartDir, path))
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}

			return nil
		},
	)
	err = tarWriter.Close()
	if err != nil {
		return nil, err
	}
	err = gzWriter.Close()
	if err != nil {
		return nil, err
	}
	if walkDirErr != nil {
		return nil, err
	}
	return archive, nil
}

func ReplaceTemplate(
	testProject string,
	repo *gittest.LocalGitRepository,
	repoURL string,
) error {
	releasesFilePath := filepath.Join(
		testProject,
		"infra",
		"prometheus",
		"releases.cue",
	)

	releasesContent, err := os.ReadFile(releasesFilePath)
	if err != nil {
		return err
	}

	tmpl, err := template.New("releases").Parse(string(releasesContent))
	if err != nil {
		return err
	}

	releasesFile, err := os.Create(releasesFilePath)
	if err != nil {
		return err
	}
	defer releasesFile.Close()

	err = tmpl.Execute(releasesFile, struct {
		Name    string
		RepoUrl string
		Version string
	}{
		Name:    "test",
		RepoUrl: repoURL,
		Version: "1.0.0",
	})
	if err != nil {
		return err
	}

	_, err = repo.CommitFile("infra/prometheus/releases.cue", "overwrite template")
	if err != nil {
		return err
	}

	return nil
}
