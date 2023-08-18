package kubetest

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-git/go-git/v5"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/gittest"
	"sigs.k8s.io/yaml"

	"github.com/kharf/declcd/pkg/kube"
	_ "github.com/kharf/declcd/test/workingdir"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type KubetestEnv struct {
	ControlPlane      *envtest.Environment
	ControllerManager manager.Manager
	HelmConfig        *action.Configuration
	HelmRepoServer    *httptest.Server
	HelmChartServer   *httptest.Server
	GitRepository     *gittest.LocalGitRepository
	TestRoot          string
	TestProject       string
	TestProjectSource string
	TestKubeClient    client.Client
	Ctx               context.Context
	clean             func()
}

func (env KubetestEnv) Stop() {
	env.clean()
}

func StartKubetestEnv(t *testing.T) *KubetestEnv {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatal(err)
	}

	err = gitopsv1.AddToScheme(scheme.Scheme)
	if err != nil {
		t.Fatal(err)
	}

	//+kubebuilder:scaffold:scheme

	k8sClient, err := kube.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	helmCfg := action.Configuration{}
	getter := kube.InMemoryRESTClientGetter{
		Cfg:        cfg,
		RestMapper: k8sClient.RestMapper,
	}
	err = helmCfg.Init(getter, "default", "secret", log.Printf)
	if err != nil {
		t.Fatal(err)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
	})
	if err != nil {
		t.Fatal(err)
	}

	helmEnv := setupHelm(t)
	server := helmEnv.repoServer
	chartServer := helmEnv.chartServer

	testRoot := filepath.Join(os.TempDir(), "decl")
	testProjectSource := filepath.Join(testRoot, "simple")
	testProject, err := os.MkdirTemp(testRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = git.PlainClone(
		testProject, false,
		&git.CloneOptions{URL: testProjectSource, Progress: os.Stdout},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.TODO())

	repo, err := gittest.OpenGitRepository(testProject)
	if err != nil {
		t.Fatal(err)
	}

	replaceTemplate(t, repo, testProject, server.URL)

	testClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatal(err)
	}
	return &KubetestEnv{
		ControlPlane:      testEnv,
		ControllerManager: mgr,
		HelmConfig:        &helmCfg,
		HelmRepoServer:    server,
		HelmChartServer:   chartServer,
		GitRepository:     repo,
		TestRoot:          testRoot,
		TestProject:       testProject,
		TestProjectSource: testProjectSource,
		TestKubeClient:    testClient,
		Ctx:               ctx,
		clean: func() {
			testEnv.Stop()
			server.Close()
			chartServer.Close()
			for _, f := range helmEnv.chartArchives {
				os.Remove(f.Name())
			}
			cancel()
		},
	}
}

func replaceTemplate(t *testing.T, repo *gittest.LocalGitRepository, testProject string, repoURL string) {
	releasesFilePath := filepath.Join(testProject, "infra", "prometheus", "releases.cue")
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

type helmEnv struct {
	chartServer   *httptest.Server
	repoServer    *httptest.Server
	chartArchives []*os.File
}

func setupHelm(t *testing.T) helmEnv {
	v1Archive := createChartArchive(t, "test", "1.0.0")
	v2Archive := createChartArchive(t, "testv2", "2.0.0")
	chartServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						URLs: []string{chartServer.URL + "/test-1.0.0.tgz"},
					},
					&repo.ChartVersion{
						Metadata: &chart.Metadata{
							APIVersion: "v1",
							Version:    "2.0.0",
							Name:       "test",
						},
						URLs: []string{chartServer.URL + "/test-2.0.0.tgz"},
					},
				},
			},
		}
		indexBytes, err := yaml.Marshal(index)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(indexBytes)
	}))

	return helmEnv{
		chartServer:   chartServer,
		repoServer:    server,
		chartArchives: []*os.File{v1Archive, v2Archive},
	}
}

func createChartArchive(t *testing.T, chart string, version string) *os.File {
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
	walkDirErr := fs.WalkDir(os.DirFS(chartDir), chart, func(path string, d fs.DirEntry, err error) error {
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
	})
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
