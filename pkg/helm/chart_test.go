package helm

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"sigs.k8s.io/yaml"
)

func actionConfigFixture(t *testing.T) *action.Configuration {
	t.Helper()

	registryClient, err := registry.NewClient()
	if err != nil {
		t.Fatal(err)
	}

	k8sMajorVersion := "1"
	k8sMinorVersion := "27"

	return &action.Configuration{
		Releases:   storage.Init(driver.NewMemory()),
		KubeClient: &fake.FailingKubeClient{PrintingKubeClient: fake.PrintingKubeClient{Out: io.Discard}},
		Capabilities: &chartutil.Capabilities{
			KubeVersion: chartutil.KubeVersion{
				Version: fmt.Sprintf("v%s.%s.0", k8sMajorVersion, k8sMinorVersion),
				Major:   k8sMajorVersion,
				Minor:   k8sMinorVersion,
			},
			APIVersions: chartutil.DefaultVersionSet,
			HelmVersion: chartutil.DefaultCapabilities.HelmVersion,
		},
		RegistryClient: registryClient,
		Log: func(format string, v ...interface{}) {
			t.Helper()
		},
	}
}

func TestChartInstaller(t *testing.T) {
	archive, err := os.CreateTemp("", "*-test-0.1.0.tgz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(archive.Name())

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gzWriter := gzip.NewWriter(archive)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	chartDir := filepath.Join(dir, "testdata", "charts")
	err = fs.WalkDir(os.DirFS(chartDir), "test", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || path == ".helmignore" {
			return nil
		}
		fmt.Println(path)

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
	if err != nil {
		t.Fatal(err)
	}

	err = tarWriter.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = gzWriter.Close()
	if err != nil {
		t.Fatal(err)
	}

	chartServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer chartServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := &repo.IndexFile{
			APIVersion: "v1",
			Generated:  time.Now(),
			Entries: map[string]repo.ChartVersions{
				"test": {
					&repo.ChartVersion{
						Metadata: &chart.Metadata{
							APIVersion: "v1",
							Version:    "0.1.0",
							Name:       "test",
						},
						URLs: []string{chartServer.URL + "/test-0.1.0.tgz"},
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
	defer server.Close()

	actionConfig := actionConfigFixture(t)
	chartInstaller := ChartInstaller{
		cfg: *actionConfig,
	}

	chart := Chart{
		name:    "test",
		version: "0.1.0",
	}

	release, err := chartInstaller.run(server.URL, chart)
	assert.NilError(t, err)
	assert.Equal(t, release.Name, "test")
}
