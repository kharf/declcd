package helm

import (
	"errors"
	"fmt"
	"os"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
)

var (
	ErrNoChartURLs      = errors.New("helm chart does not provide download urls")
	ErrPullFailed       = errors.New("could not pull helm chart")
	ErrHelmChartVersion = errors.New("helm chart version error")
)

type Chart struct {
	name    string
	version string
}

type ChartInstaller struct {
	cfg action.Configuration
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

func (c ChartInstaller) run(repoURL string, chart Chart, opts ...option) (*release.Release, error) {
	client := action.NewInstall(&c.cfg)
	client.Wait = true

	installOpts := &options{}
	for _, opt := range opts {
		opt.Apply(installOpts)
	}

	releaseName := chart.name
	if installOpts.releaseName != "" {
		releaseName = installOpts.releaseName
	}
	client.ReleaseName = releaseName

	namespace := "default"
	if installOpts.namespace != "" {
		namespace = installOpts.namespace
	}
	client.Namespace = namespace

	chrt, err := pull(repoURL, chart)
	if err != nil {
		return nil, err
	}

	release, err := client.Run(chrt, installOpts.values)
	if err != nil {
		return nil, err
	}

	return release, nil
}

func pull(repoURL string, chartRequest Chart) (*chart.Chart, error) {
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
		URL:  repoURL,
		Name: chartRequest.name,
	}
	chartRepo, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return nil, err
	}
	path, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return nil, err
	}

	index, err := repo.LoadIndexFile(path)
	if err != nil {
		return nil, err
	}

	chartVersion, err := index.Get(chartRequest.name, chartRequest.version)
	if err != nil {
		return nil, fmt.Errorf("%w: version: %s not found: %w", ErrHelmChartVersion, chartRequest.version, err)
	}

	if len(chartVersion.URLs) < 1 {
		return nil, ErrNoChartURLs
	}

	absoluteChartURL, err := repo.ResolveReferenceURL(repoURL, chartVersion.URLs[0])
	if err != nil {
		return nil, err
	}

	dest, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}

	chartPath, _, err := chartDownloader.DownloadTo(absoluteChartURL, chartRequest.version, dest)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPullFailed, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}

	return chart, nil
}
