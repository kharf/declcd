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
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
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
	chartIdentifier := fmt.Sprintf("%s-%s", chartRequest.Name, chartRequest.Version)
	chartPath := filepath.Join(os.TempDir(), chartIdentifier)
	chartArchivePath := filepath.Join(chartPath, fmt.Sprintf("%s.tgz", chartIdentifier))
	chart, err := loader.Load(chartArchivePath)
	if err != nil {
		pathErr := &fs.PathError{}
		if errors.As(err, &pathErr) {
			c.Log.Info("pulling chart", logArgs...)
			if err := c.pull(chartRequest, chartPath); err != nil {
				return nil, err
			}
			chart, err := loader.Load(chartArchivePath)
			if err != nil {
				return nil, err
			}
			return chart, nil
		}
		return nil, err
	}

	return chart, nil
}

func (c ChartReconciler) pull(chartRequest Chart, chartPath string) error {
	var err error

	getters := []getter.Provider{
		{
			Schemes: []string{"http", "https"},
			New:     getter.NewHTTPGetter,
		},
	}
	chartDownloader := downloader.ChartDownloader{
		Out:     os.Stdout,
		Getters: getters,
	}
	entry := &repo.Entry{
		URL:  chartRequest.RepoURL,
		Name: chartRequest.Name,
	}
	chartRepo, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return err
	}
	path, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return err
	}

	index, err := repo.LoadIndexFile(path)
	if err != nil {
		return err
	}

	chartVersion, err := index.Get(chartRequest.Name, chartRequest.Version)
	if err != nil {
		return fmt.Errorf("%w: version: %s not found: %w", ErrHelmChartVersion, chartRequest.Version, err)
	}

	if len(chartVersion.URLs) < 1 {
		return ErrNoChartURLs
	}

	absoluteChartURL, err := repo.ResolveReferenceURL(chartRequest.RepoURL, chartVersion.URLs[0])
	if err != nil {
		return err
	}

	err = os.Mkdir(chartPath, 0700)
	if err != nil {
		return err
	}

	_, _, err = chartDownloader.DownloadTo(absoluteChartURL, chartRequest.Version, chartPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPullFailed, err)
	}

	return nil
}
