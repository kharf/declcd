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

// A Helm package that contains information sufficient for installing a set of Kubernetes resources into a Kubernetes cluster.
type Chart struct {
	Name string `json:"name"`
	// URL of the repository where the Helm chart is hosted.
	RepoURL string `json:"repoURL"`
	Version string `json:"version"`
}

// ChartReconciler reads Helm Packages with their desired state and applies them on a Kubernetes cluster.
type ChartReconciler struct {
	// Configuration used for Helm operations
	Cfg action.Configuration
	Log logr.Logger
}

type options struct {
	releaseName string
	namespace   string
	values      map[string]interface{}
}

// Option is a specific configuration used for reconciling Helm Charts.
type option interface {
	Apply(opts *options)
}

// ReleaseName influences the name of the installed objects of a Helm Chart.
// When set, the installed objects are suffixed with the chart name.
// Defaults to the chart name.
type ReleaseName string

func (r ReleaseName) Apply(opts *options) {
	opts.releaseName = string(r)
}

// Namespaces specifies the Kubernetes namespace to which the Helm Chart is installed to.
// Defaults to default.
type Namespace string

func (n Namespace) Apply(opts *options) {
	opts.namespace = string(n)
}

// Values provide a way to override Helm Chart template defaults with your own information.
type Values map[string]interface{}

func (v Values) Apply(opts *options) {
	opts.values = v
}

// Reconcile reads a declared Helm Chart with its desired state and applies it on a Kubernetes cluster.
// It upgrades a Helm Chart based on whether it is already installed or not.
// In case an upgrade or installation is interrupted and left in a dangling state, the dangling release secret will be removed and a new upgrade/installation will be run.
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
	c.Log.Info("Loading chart", logArgs...)
	chrt, err := c.load(chartRequest, logArgs)
	if err != nil {
		return nil, err
	}
	histClient := action.NewHistory(&c.Cfg)
	histClient.Max = 1
	releases, err := histClient.Run(releaseName)
	if err != nil {
		if err != driver.ErrReleaseNotFound {
			return nil, err
		}
		return c.install(releaseName, namespace, logArgs, chrt, reconcileOpts)
	}
	if len(releases) == 1 {
		if releases[0].Info.Status == release.StatusPendingInstall {
			if err := c.reset(releases[0], logArgs); err != nil {
				return nil, err
			}
			return c.install(releaseName, namespace, logArgs, chrt, reconcileOpts)
		}
	}
	upgrade := action.NewUpgrade(&c.Cfg)
	upgrade.Wait = false
	upgrade.Namespace = namespace
	upgrade.MaxHistory = 5
	runUpgrade := func() (*release.Release, error) {
		c.Log.Info("Upgrading release", logArgs...)
		return upgrade.Run(releaseName, chrt, reconcileOpts.values)
	}
	release, err := runUpgrade()
	if err != nil {
		release := releases[len(releases)-1]
		if release.Info.Status.IsPending() {
			if err := c.reset(release, logArgs); err != nil {
				return nil, err
			}
			return runUpgrade()
		} else {
			return nil, err
		}
	}
	return release, nil
}

func (c ChartReconciler) install(releaseName string, namespace string, logArgs []interface{}, chrt *chart.Chart, reconcileOpts *options) (*release.Release, error) {
	install := action.NewInstall(&c.Cfg)
	install.Wait = false
	install.ReleaseName = releaseName
	install.CreateNamespace = true
	install.Namespace = namespace
	c.Log.Info("Installing chart", logArgs...)
	release, err := install.Run(chrt, reconcileOpts.values)
	if err != nil {
		c.Log.Error(err, "Installing chart failed", logArgs...)
		return nil, err
	}
	return release, nil
}

func (c ChartReconciler) reset(release *release.Release, logArgs []interface{}) error {
	c.Log.Info("Resetting dangling release", logArgs...)
	_, err := c.Cfg.Releases.Delete(release.Name, release.Version)
	if err != nil {
		c.Log.Error(err, "Resetting dangling release failed", logArgs...)
		return err
	}
	return nil
}

func (c ChartReconciler) load(chartRequest Chart, logArgs []interface{}) (*chart.Chart, error) {
	var err error
	archivePath := newArchivePath(chartRequest)
	chart, err := loader.Load(archivePath.fullPath)
	if err != nil {
		pathErr := &fs.PathError{}
		if errors.As(err, &pathErr) {
			c.Log.Info("Pulling chart", logArgs...)
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

// Remove removes the locally stored Helm Chart from the file system, but does not uninstall the Chart/Release.
func Remove(chart Chart) error {
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
