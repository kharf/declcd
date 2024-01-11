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

	"gotest.tools/v3/assert"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/google/go-containerregistry/pkg/registry"
	gitopsv1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/internal/gittest"
	helmRegistry "helm.sh/helm/v3/pkg/registry"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/garbage"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/secret"
	_ "github.com/kharf/declcd/test/workingdir"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type KubetestEnv struct {
	ControlPlane          *envtest.Environment
	ControllerManager     manager.Manager
	HelmEnv               helmEnv
	TestKubeClient        client.Client
	DynamicTestKubeClient *kube.DynamicClient
	GarbageCollector      garbage.Collector
	InventoryManager      inventory.Manager
	Manager               secret.Manager
	Ctx                   context.Context
	clean                 func()
}

func (env KubetestEnv) Stop() {
	env.clean()
}

type helmOption struct {
	enabled bool
	oci     bool
}

var _ Option = (*helmOption)(nil)

func (opt helmOption) apply(opts *options) {
	opts.helm = opt
}

type projectOption struct {
	repo        *gittest.LocalGitRepository
	testProject string
	testRoot    string
}

var _ Option = (*projectOption)(nil)

func (opt projectOption) apply(opts *options) {
	opts.project = opt
}

type kubernetesDisabled bool

var _ Option = (*kubernetesDisabled)(nil)

func (opt kubernetesDisabled) apply(opts *options) {
	opts.enabled = bool(opt)
}

type decryptionKeyCreated bool

var _ Option = (*decryptionKeyCreated)(nil)

func (opt decryptionKeyCreated) apply(opts *options) {
	opts.decryptionKeyCreated = bool(opt)
}

type options struct {
	enabled              bool
	helm                 helmOption
	decryptionKeyCreated bool
	project              projectOption
}

type Option interface {
	apply(*options)
}

func WithKubernetesDisabled() kubernetesDisabled {
	return false
}

func WithHelm(enabled bool, oci bool) helmOption {
	return helmOption{
		enabled: enabled,
		oci:     oci,
	}
}

func WithDecryptionKeyCreated() decryptionKeyCreated {
	return true
}

// Has no effect when provided to projecttest.StartProjectEnv
func WithProject(repo *gittest.LocalGitRepository, testProject string, testRoot string) projectOption {
	return projectOption{
		repo:        repo,
		testProject: testProject,
		testRoot:    testRoot,
	}
}

func StartKubetestEnv(t *testing.T, log logr.Logger, opts ...Option) *KubetestEnv {
	options := &options{
		helm: helmOption{
			enabled: false,
			oci:     false,
		},
		enabled:              true,
		decryptionKeyCreated: false,
	}
	for _, o := range opts {
		o.apply(options)
	}
	if !options.enabled {
		return nil
	}
	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: false,
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
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.TODO())
	testClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatal(err)
	}
	helmEnv := setupHelm(t, cfg, options)
	client, err := kube.NewDynamicClient(testEnv.Config)
	assert.NilError(t, err)
	inventoryPath, err := os.MkdirTemp(options.project.testRoot, "inventory-*")
	assert.NilError(t, err)
	invManager := inventory.Manager{
		Log:  log,
		Path: inventoryPath,
	}
	gc := garbage.Collector{
		Log:              log,
		Client:           client,
		InventoryManager: invManager,
		HelmConfig:       helmEnv.HelmConfig,
	}
	nsStr := "test"
	if options.decryptionKeyCreated {
		privKey := "AGE-SECRET-KEY-1EYUZS82HMQXK0S83AKAP6NJ7HPW6KMV70DHHMH4TS66S3NURTWWS034Q34"
		declNs := corev1.Namespace{
			TypeMeta: v1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name: nsStr,
			},
		}
		err = testClient.Create(ctx, &declNs)
		assert.NilError(t, err)
		decSec := corev1.Secret{
			TypeMeta: v1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      secret.K8sSecretName,
				Namespace: nsStr,
			},
			Data: map[string][]byte{
				secret.K8sSecretDataKey: []byte(privKey),
			},
		}
		err = testClient.Create(ctx, &decSec)
		assert.NilError(t, err)
	}
	manager := secret.NewManager(options.project.testProject, nsStr, client)
	return &KubetestEnv{
		ControlPlane:          testEnv,
		ControllerManager:     mgr,
		HelmEnv:               helmEnv,
		TestKubeClient:        testClient,
		DynamicTestKubeClient: client,
		GarbageCollector:      gc,
		InventoryManager:      invManager,
		Manager:               manager,
		Ctx:                   ctx,
		clean: func() {
			testEnv.Stop()
			helmEnv.Close()
			cancel()
		},
	}
}

func replaceTemplate(t *testing.T, options *options, repoURL string) {
	releasesFilePath := filepath.Join(options.project.testProject, "infra", "prometheus", "releases.cue")
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

	err = options.project.repo.CommitFile("infra/prometheus/releases.cue", "overwrite template")
	if err != nil {
		t.Fatal(err)
	}
}

type helmEnv struct {
	HelmConfig       action.Configuration
	ChartServer      *httptest.Server
	RepositoryServer *httptest.Server
	chartArchives    []*os.File
}

func (env helmEnv) Close() {
	if env.ChartServer != nil {
		env.ChartServer.Close()
	}
	if env.RepositoryServer != nil {
		env.RepositoryServer.Close()
	}
	for _, f := range env.chartArchives {
		os.Remove(f.Name())
	}
}

func setupHelm(t *testing.T, cfg *rest.Config, options *options) helmEnv {
	helmCfg := action.Configuration{}
	var helmEnv helmEnv
	if options.helm.enabled {
		k8sClient, err := kube.NewDynamicClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		getter := kube.InMemoryRESTClientGetter{
			Cfg:        cfg,
			RestMapper: k8sClient.RestMapper,
		}
		err = helmCfg.Init(getter, "default", "secret", log.Printf)
		if err != nil {
			t.Fatal(err)
		}
		helmEnv = startHelmServer(t, options.helm.oci)
		replaceTemplate(t, options, helmEnv.RepositoryServer.URL)
	}
	// need to be always set, even though we dont test helm releases
	helmEnv.HelmConfig = helmCfg
	return helmEnv
}

func startHelmServer(t *testing.T, oci bool) helmEnv {
	v1Archive := createChartArchive(t, "test", "1.0.0")
	v2Archive := createChartArchive(t, "testv2", "2.0.0")
	var chartServer *httptest.Server
	if oci {
		opts := []registry.Option{registry.WithReferrersSupport(true)}
		chartServer = httptest.NewServer(registry.New(opts...))
		helmRegistryClient, err := helmRegistry.NewClient()
		assert.NilError(t, err)
		v1Bytes, err := os.ReadFile(v1Archive.Name())
		assert.NilError(t, err)
		ociRepo, found := strings.CutPrefix(chartServer.URL, "http://")
		assert.Assert(t, found)
		_, err = helmRegistryClient.Push(v1Bytes, fmt.Sprintf("%s/%s:%s", ociRepo, "test", "1.0.0"))
		assert.NilError(t, err)
	} else {
		chartServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}
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
		ChartServer:      chartServer,
		RepositoryServer: server,
		chartArchives:    []*os.File{v1Archive, v2Archive},
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

type FakeDynamicClient struct {
	Err error
}

var _ kube.Client[unstructured.Unstructured] = (*FakeDynamicClient)(nil)

func (client *FakeDynamicClient) Apply(ctx context.Context, obj *unstructured.Unstructured, fieldManager string) error {
	return client.Err
}

func (client *FakeDynamicClient) Delete(ctx context.Context, obj *unstructured.Unstructured) error {
	return client.Err
}

func (client *FakeDynamicClient) Get(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return nil, client.Err
}
