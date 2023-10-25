package helm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

var (
	ErrNoChartURLs      = errors.New("helm chart does not provide download urls")
	ErrPullFailed       = errors.New("could not pull helm chart")
	ErrHelmChartVersion = errors.New("helm chart version error")
)

type Chart struct {
	Name    string `json:"name"`
	RepoURL string `json:"repoURL"`
	Version string `json:"version"`
}

type ChartReconciler struct {
	Cfg action.Configuration
	Log logr.Logger
}

type options struct {
	releaseName string
	namespace   string
	values      map[string]interface{}
}

type option interface {
	Apply(opts *options)
}

type ReleaseName string

func (r ReleaseName) Apply(opts *options) {
	opts.releaseName = string(r)
}

type Namespace string

func (n Namespace) Apply(opts *options) {
	opts.namespace = string(n)
}

type Values map[string]interface{}

func (v Values) Apply(opts *options) {
	opts.values = v
}

func (c ChartReconciler) Reconcile(chartRequest Chart, opts ...option) (*release.Release, error) {
	reconcileOpts := &options{}
	for _, opt := range opts {
		opt.Apply(reconcileOpts)
	}
	releaseName := chartRequest.Name
	if reconcileOpts.releaseName != "" {
		releaseName = reconcileOpts.releaseName
	}
	namespace := "default"
	if reconcileOpts.namespace != "" {
		namespace = reconcileOpts.namespace
	}
	logArgs := []interface{}{"name", chartRequest.Name, "url", chartRequest.RepoURL, "version", chartRequest.Version, "releasename", releaseName, "namespace", namespace}
	c.Log.Info("loading chart", logArgs...)
	chrt, err := c.load(chartRequest, logArgs)
	if err != nil {
		return nil, err
	}

	histClient := action.NewHistory(&c.Cfg)
	histClient.Max = 1
	if _, err := histClient.Run(releaseName); err == driver.ErrReleaseNotFound {
		client := action.NewInstall(&c.Cfg)
		client.Wait = false
		client.ReleaseName = releaseName
		client.CreateNamespace = true
		client.Namespace = namespace
		c.Log.Info("installing chart", logArgs...)
		release, err := client.Run(chrt, reconcileOpts.values)
		if err != nil {
			c.Log.Error(err, "installing chart failed", logArgs...)
			return nil, err
		}
		c.Log.Info("installing chart finished", logArgs...)
		return release, nil
	}
	upgrade := action.NewUpgrade(&c.Cfg)
	upgrade.Wait = false
	upgrade.Namespace = namespace
	upgrade.MaxHistory = 5
	c.Log.Info("upgrading chart", logArgs...)
	release, err := upgrade.Run(releaseName, chrt, reconcileOpts.values)
	if err != nil {
		c.Log.Error(err, "upgrading chart failed", logArgs...)
		return nil, err
	}
	c.Log.Info("upgrading chart finished", logArgs...)
	return release, nil
}

func (c ChartReconciler) load(chartRequest Chart, logArgs []interface{}) (*chart.Chart, error) {
	var err error
	archivePath := newArchivePath(chartRequest)
	chart, err := loader.Load(archivePath.fullPath)
	if err != nil {
		pathErr := &fs.PathError{}
		if errors.As(err, &pathErr) {
			c.Log.Info("pulling chart", logArgs...)
			if err := c.pull(chartRequest, archivePath.dir); err != nil {
				return nil, err
			}
			chart, err := loader.Load(archivePath.fullPath)
			if err != nil {
				return nil, err
			}
			return chart, nil
		}
		return nil, err
	}

	return chart, nil
}

func (c ChartReconciler) pull(chartRequest Chart, chartDestPath string) error {
	pull := action.NewPullWithOpts(action.WithConfig(&c.Cfg))
	pull.DestDir = chartDestPath
	var chartRef string
	if registry.IsOCI(chartRequest.RepoURL) {
		chartRef = fmt.Sprintf("%s/%s", chartRequest.RepoURL, chartRequest.Name)
	} else {
		pull.RepoURL = chartRequest.RepoURL
		chartRef = chartRequest.Name
	}
	pull.Settings = cli.New()
	pull.Version = chartRequest.Version
	err := os.MkdirAll(chartDestPath, 0700)
	if err != nil {
		return err
	}
	opts := []registry.ClientOption{
		registry.ClientOptDebug(false),
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(os.Stderr),
	}
	registryClient, err := registry.NewClient(opts...)
	if err != nil {
		return err
	}
	pull.SetRegistryClient(registryClient)

	_, err = pull.Run(chartRef)
	if err != nil {
		return err
	}

	return nil
}

func remove(chart Chart) error {
	return os.RemoveAll(newArchivePath(chart).fullPath)
}

type archivePath struct {
	dir      string
	fullPath string
}

func newArchivePath(chart Chart) archivePath {
	chartIdentifier := fmt.Sprintf("%s-%s", chart.Name, chart.Version)
	chartDestPath := filepath.Join(os.TempDir(), chart.Name)
	fullPath := filepath.Join(chartDestPath, fmt.Sprintf("%s.tgz", chartIdentifier))
	return archivePath{
		dir:      chartDestPath,
		fullPath: fullPath,
	}
}
