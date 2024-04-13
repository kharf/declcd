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
	"testing"
	"text/template"
	"time"

	"github.com/kharf/declcd/internal/gittest"
	"github.com/kharf/declcd/internal/ocitest"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/project"
	"gotest.tools/v3/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	helmKube "helm.sh/helm/v3/pkg/kube"
	helmRegistry "helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type projectOption struct {
	repo        *gittest.LocalGitRepository
	testProject string
	testRoot    string
}

var _ Option = (*projectOption)(nil)

func (opt projectOption) apply(opts *options) {
	opts.project = opt
}

type oci bool

var _ Option = (*oci)(nil)

func (opt oci) apply(opts *options) {
	opts.oci = bool(opt)
}

type options struct {
	oci     bool
	project projectOption
}

type Option interface {
	apply(*options)
}

func WithOCI(enabled bool) oci {
	return oci(enabled)
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

type Server interface {
	// base URL of form http://ipaddr:port with no trailing slash
	URL() string
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

type Environment struct {
	HelmConfig    action.Configuration
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

func StartHelmEnv(t testing.TB, cfg *rest.Config, opts ...Option) Environment {
	options := &options{
		oci: false,
	}
	for _, o := range opts {
		o.apply(options)
	}
	helmCfg := action.Configuration{}
	var helmEnv Environment
	helmKube.ManagedFieldsManager = project.ControllerName
	k8sClient, err := kube.NewDynamicClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	getter := &kube.InMemoryRESTClientGetter{
		Cfg:        cfg,
		RestMapper: k8sClient.RESTMapper(),
	}
	err = helmCfg.Init(getter, "default", "secret", log.Printf)
	if err != nil {
		t.Fatal(err)
	}
	helmCfg.KubeClient = &kube.HelmClient{
		Client:        helmCfg.KubeClient.(*helmKube.Client),
		DynamicClient: k8sClient,
		FieldManager:  "controller",
	}
	helmEnv = startHelmServer(t, options)
	ReplaceTemplate(t, options.project.testProject, options.project.repo, helmEnv.ChartServer.URL())
	// need to be always set, even though we dont test helm releases
	helmEnv.HelmConfig = helmCfg
	return helmEnv
}

func startHelmServer(t testing.TB, options *options) Environment {
	v1Archive := createChartArchive(t, "test", "1.0.0")
	v2Archive := createChartArchive(t, "testv2", "2.0.0")
	var chartServer Server
	if options.oci {
		ociServer, err := ocitest.NewTLSRegistry()
		assert.NilError(t, err)
		helmOpts := []helmRegistry.ClientOption{
			helmRegistry.ClientOptDebug(true),
			helmRegistry.ClientOptWriter(os.Stderr),
			helmRegistry.ClientOptHTTPClient(ociServer.Client()),
			helmRegistry.ClientOptResolver(nil),
		}
		helmRegistryClient, err := helmRegistry.NewClient(helmOpts...)
		assert.NilError(t, err)
		v1Bytes, err := os.ReadFile(v1Archive.Name())
		assert.NilError(t, err)
		_, err = helmRegistryClient.Push(
			v1Bytes,
			fmt.Sprintf("%s/%s:%s", ociServer.Addr(), "test", "1.0.0"),
		)
		assert.NilError(t, err)
		chartServer = &ociRegistry{
			server: ociServer,
		}
	} else {
		httpsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					t.Fatal(err)
				}
				w.Write(indexBytes)
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
				t.Fatal(err)
			}
			if _, err := io.Copy(w, file); err != nil {
				t.Fatal(err)
			}
		}))
		chartServer = &yamlBasedRepository{
			server: httpsServer,
		}
	}
	return Environment{
		ChartServer:   chartServer,
		chartArchives: []*os.File{v1Archive, v2Archive},
	}
}

func createChartArchive(t testing.TB, chart string, version string) *os.File {
	archive, err := os.CreateTemp("", fmt.Sprintf("*-test-%s.tgz", version))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gzWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzWriter)
	chartDir := filepath.Join(dir, "test", "testdata", "charts")
	walkDirErr := fs.WalkDir(
		os.DirFS(chartDir),
		chart,
		func(path string, d fs.DirEntry, err error) error {
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
		t.Fatal(err)
	}
	err = gzWriter.Close()
	if err != nil {
		t.Fatal(err)
	}
	if walkDirErr != nil {
		t.Fatal(err)
	}
	return archive
}

func ReplaceTemplate(
	t testing.TB,
	testProject string,
	repo *gittest.LocalGitRepository,
	repoURL string,
) {
	releasesFilePath := filepath.Join(
		testProject,
		"infra",
		"prometheus",
		"releases.cue",
	)
	releasesContent, err := os.ReadFile(releasesFilePath)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("releases").Parse(string(releasesContent))
	if err != nil {
		t.Fatal(err)
	}
	releasesFile, err := os.Create(releasesFilePath)
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	err = repo.CommitFile("infra/prometheus/releases.cue", "overwrite template")
	if err != nil {
		t.Fatal(err)
	}
}
