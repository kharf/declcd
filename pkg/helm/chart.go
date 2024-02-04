package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// A Helm package that contains information
// sufficient for installing a set of Kubernetes resources into a Kubernetes cluster.
type Chart struct {
	Name string `json:"name"`
	// URL of the repository where the Helm chart is hosted.
	RepoURL string `json:"repoURL"`
	Version string `json:"version"`
}

// ChartReconciler reads Helm Packages with their desired state
// and applies them on a Kubernetes cluster.
// It stores releases in the inventory, but never collects it.
type ChartReconciler struct {
	// Configuration used for Helm operations
	cfg              action.Configuration
	client           kube.Client[unstructured.Unstructured]
	fieldManager     string
	inventoryManager inventory.Manager
	log              logr.Logger
}

// NewChartReconciler constructs a ChartReconciler,
// which reads Helm Packages with their desired state and applies them on a Kubernetes cluster.
func NewChartReconciler(cfg action.Configuration, client kube.Client[unstructured.Unstructured], fieldManager string, inventoryManager inventory.Manager, log logr.Logger) ChartReconciler {
	return ChartReconciler{
		cfg:              cfg,
		client:           client,
		fieldManager:     fieldManager,
		inventoryManager: inventoryManager,
		log:              log,
	}
}

// Reconcile reads a declared Helm Release with its desired state and applies it on a Kubernetes cluster.
// It upgrades a Helm Chart based on whether it is already installed or not.
// A successful run stores the release in the inventory, but never collects it.
// In case an upgrade or installation is interrupted and left in a dangling state, the dangling release secret will be removed and a new upgrade/installation will be run.
func (c ChartReconciler) Reconcile(ctx context.Context, componentID string, desiredRelease ReleaseDeclaration) (*Release, error) {
	if desiredRelease.Name == "" {
		desiredRelease.Name = desiredRelease.Chart.Name
	}
	if desiredRelease.Namespace == "" {
		desiredRelease.Namespace = "default"
	}
	installedRelease, err := c.doReconcile(ctx, componentID, desiredRelease)
	if err != nil {
		return nil, err
	}
	invRelease := inventory.NewHelmReleaseItem(
		componentID,
		installedRelease.Name,
		installedRelease.Namespace,
	)
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(installedRelease); err != nil {
		return nil, err
	}
	if err := c.inventoryManager.StoreItem(invRelease, buf); err != nil {
		return nil, err
	}
	return installedRelease, nil
}

func (c ChartReconciler) doReconcile(ctx context.Context, componentID string, desiredRelease ReleaseDeclaration) (*Release, error) {
	logArgs := []interface{}{"name", desiredRelease.Chart.Name, "url", desiredRelease.Chart.RepoURL, "version", desiredRelease.Chart.Version, "releasename", desiredRelease.Name, "namespace", desiredRelease.Namespace}
	c.log.Info("Loading chart", logArgs...)
	chrt, err := c.load(desiredRelease.Chart, logArgs)
	if err != nil {
		return nil, err
	}
	histClient := action.NewHistory(&c.cfg)
	histClient.Max = 2
	releases, err := histClient.Run(desiredRelease.Name)
	if err != nil {
		if err != driver.ErrReleaseNotFound {
			return nil, err
		}
		return c.install(desiredRelease, chrt, logArgs)
	}
	if len(releases) == 1 {
		if releases[0].Info.Status == release.StatusPendingInstall {
			if err := c.reset(releases[0], logArgs); err != nil {
				return nil, err
			}
			return c.install(desiredRelease, chrt, logArgs)
		}
	}
	driftType, err := c.diff(ctx, componentID, desiredRelease, chrt, logArgs)
	if err != nil {
		release := releases[len(releases)-1]
		if !release.Info.Status.IsPending() {
			return nil, err
		}
		if err := c.reset(release, logArgs); err != nil {
			return nil, err
		}
		driftType = driftTypeUpdate
	}
	if driftType == driftTypeNone {
		c.log.Info("No changes", logArgs...)
		latestInternalRelease := releases[len(releases)-1]
		return &Release{
			Name:      latestInternalRelease.Name,
			Namespace: latestInternalRelease.Namespace,
			Chart:     desiredRelease.Chart,
			Values:    desiredRelease.Values,
			Version:   latestInternalRelease.Version,
		}, nil
	}
	upgrade := action.NewUpgrade(&c.cfg)
	upgrade.Wait = false
	upgrade.Namespace = desiredRelease.Namespace
	upgrade.MaxHistory = 5
	if driftType == driftTypeConflict {
		upgrade.Force = true
	}
	c.log.Info("Upgrading release", logArgs...)
	release, err := upgrade.Run(desiredRelease.Name, chrt, desiredRelease.Values)
	if err != nil {
		return nil, err
	}
	return &Release{
		Name:      release.Name,
		Namespace: release.Namespace,
		Chart:     desiredRelease.Chart,
		Values:    desiredRelease.Values,
		Version:   release.Version,
	}, nil
}

type driftType string

const (
	driftTypeConflict driftType = "conflict"
	driftTypeDeleted            = "deleted"
	driftTypeUpdate             = "update"
	driftTypeNone               = "none"
)

func (c ChartReconciler) diff(ctx context.Context, componentID string, desiredRelease ReleaseDeclaration, loadedChart *chart.Chart, logArgs []interface{}) (driftType, error) {
	upgrade := action.NewUpgrade(&c.cfg)
	upgrade.Wait = false
	upgrade.Namespace = desiredRelease.Namespace
	upgrade.DryRun = true
	release, err := upgrade.Run(desiredRelease.Name, loadedChart, desiredRelease.Values)
	if err != nil {
		return "", err
	}
	decoder := yaml.NewDecoder(bytes.NewBufferString(release.Manifest))
	for {
		var unstr map[string]interface{}
		if err = decoder.Decode(&unstr); err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if len(unstr) == 0 {
			continue
		}
		newManifest := &unstructured.Unstructured{Object: unstr}
		obj, err := c.client.Get(ctx, newManifest)
		if err != nil {
			switch k8sErrors.ReasonForError(err) {
			case v1.StatusReasonNotFound:
				return driftTypeDeleted, nil
			}
			return "", err
		}
		if obj == nil {
			return driftTypeDeleted, nil
		}
		if err := c.client.Apply(ctx, newManifest, c.fieldManager, kube.DryRun(true)); err != nil {
			switch k8sErrors.ReasonForError(err) {
			case v1.StatusReasonUnknown:
				return "", err
			}
			logArgs = append(logArgs, "manifest", newManifest.GetName(), "kind", newManifest.GetKind(), "apiVersion", newManifest.GetAPIVersion(), "err", err)
			c.log.V(1).Info("Drift detected", logArgs...)
			return driftTypeConflict, nil
		}
	}
	contentReader, err := c.inventoryManager.GetItem(inventory.NewHelmReleaseItem(componentID, desiredRelease.Name, desiredRelease.Namespace))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return driftTypeDeleted, nil
		}
		return "", err
	}
	defer contentReader.Close()
	storedRelease := Release{}
	if err := json.NewDecoder(contentReader).Decode(&storedRelease); err != nil {
		return "", err
	}
	if isEqual := cmp.Equal(desiredRelease, ReleaseDeclaration{
		Name:      storedRelease.Name,
		Namespace: storedRelease.Namespace,
		Chart:     storedRelease.Chart,
		Values:    storedRelease.Values,
	}); isEqual {
		return driftTypeNone, nil
	}
	return driftTypeUpdate, nil
}

func (c ChartReconciler) install(desiredRelease ReleaseDeclaration, loadedChart *chart.Chart, logArgs []interface{}) (*Release, error) {
	install := action.NewInstall(&c.cfg)
	install.Wait = false
	install.ReleaseName = desiredRelease.Name
	install.CreateNamespace = true
	install.Namespace = desiredRelease.Namespace
	c.log.Info("Installing chart", logArgs...)
	release, err := install.Run(loadedChart, desiredRelease.Values)
	if err != nil {
		c.log.Error(err, "Installing chart failed", logArgs...)
		return nil, err
	}
	return &Release{
		Name:      release.Name,
		Namespace: release.Namespace,
		Chart:     desiredRelease.Chart,
		Values:    desiredRelease.Values,
		Version:   release.Version,
	}, nil
}

func (c ChartReconciler) reset(release *release.Release, logArgs []interface{}) error {
	c.log.Info("Resetting dangling release", logArgs...)
	_, err := c.cfg.Releases.Delete(release.Name, release.Version)
	if err != nil {
		c.log.Error(err, "Resetting dangling release failed", logArgs...)
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
			c.log.Info("Pulling chart", logArgs...)
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
	pull := action.NewPullWithOpts(action.WithConfig(&c.cfg))
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

// ReleaseMetadata is a small representation of a Release.
// Release is a running instance of a Chart.
// When a chart is installed, the ChartReconciler creates a release to track that installation.
type ReleaseMetadata struct {
	componentID string
	name        string
	namespace   string
}

// NewReleaseMetadata constructs a ReleaseMetadata,
// which is a small representation of a Release.
func NewReleaseMetadata(componentID string, name string, namespace string) ReleaseMetadata {
	return ReleaseMetadata{
		componentID: componentID,
		name:        name,
		namespace:   namespace,
	}
}

// Name of the helm release.
func (hr ReleaseMetadata) Name() string {
	return hr.name
}

// Namespace of the helm release.
func (hr ReleaseMetadata) Namespace() string {
	return hr.namespace
}

// ComponentID is a link to the component this release belongs to.
func (hr ReleaseMetadata) ComponentID() string {
	return hr.componentID
}
