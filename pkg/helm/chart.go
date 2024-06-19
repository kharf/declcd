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

package helm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	helmKube "helm.sh/helm/v3/pkg/kube"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

var (
	ErrAuthSecretValueNotFound = errors.New("Auth secret value not found")
)

// SecretRef is the reference to the secret containing the repository/registry authentication.
type SecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// WorkloadIdentity is a keyless approach used for repository/registry authentication.
type WorkloadIdentity struct {
	Provider string `json:"provider"`
}

// Auth contains methods for repository/registry authentication.
type Auth struct {
	SecretRef        *SecretRef        `json:"secretRef"`
	WorkloadIdentity *WorkloadIdentity `json:"workloadIdentity"`
}

// A Helm package that contains information
// sufficient for installing a set of Kubernetes resources into a Kubernetes cluster.
type Chart struct {
	Name string `json:"name"`

	// URL of the repository where the Helm chart is hosted.
	RepoURL string `json:"repoURL"`

	Version string `json:"version"`

	// Authentication information for private repositories.
	Auth *Auth `json:"auth,omitempty"`
}

// ChartReconciler reads Helm Packages with their desired state
// and applies them on a Kubernetes cluster.
// It stores releases in the inventory, but never collects it.
type ChartReconciler struct {
	Log logr.Logger

	KubeConfig   *rest.Config
	Client       kube.Client[unstructured.Unstructured]
	FieldManager string

	// Instance is a representation of an inventory.
	// It can store, delete and read items.
	// The object does not include the storage itself, it only holds a reference to the storage.
	InventoryInstance *inventory.Instance

	// InsecureSkipVerify controls whether the Helm client verifies the server's
	// certificate chain and host name.
	InsecureSkipTLSverify bool

	// Force http for Helm registries.
	PlainHTTP bool
}

type logKey struct{}
type configKey struct{}

// Reconcile reads a declared Helm Release with its desired state and applies it on a Kubernetes cluster.
// It upgrades a Helm Chart based on whether it is already installed or not.
// A successful run stores the release in the inventory, but never collects it.
// In case an upgrade or installation is interrupted and left in a dangling state, the dangling release secret will be removed and a new upgrade/installation will be run.
func (c *ChartReconciler) Reconcile(
	ctx context.Context,
	component *ReleaseComponent,
) (*Release, error) {
	desiredRelease := component.Content
	inventoryInstance := c.InventoryInstance

	logger := c.Log.WithValues(
		"name",
		desiredRelease.Chart.Name,
		"url",
		desiredRelease.Chart.RepoURL,
		"version",
		desiredRelease.Chart.Version,
		"releasename",
		desiredRelease.Name,
		"namespace",
		desiredRelease.Namespace,
	)
	ctx = context.WithValue(ctx, logKey{}, &logger)

	if component.Content.Name == "" {
		component.Content.Name = component.Content.Chart.Name
	}
	if component.Content.Namespace == "" {
		component.Content.Namespace = "default"
	}

	// Need to init on every reconcile in order to override the fallback namespace, which is taken from the kube config
	// when templates have no metadata.namespace defined.
	helmCfg, err := Init(component.Content.Namespace, c.KubeConfig, c.Client, c.FieldManager)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, configKey{}, helmCfg)

	installedRelease, err := c.installOrUpgrade(
		ctx,
		component,
		inventoryInstance,
	)
	if err != nil {
		return nil, err
	}

	invRelease := &inventory.HelmReleaseItem{
		Name:      installedRelease.Name,
		Namespace: installedRelease.Namespace,
		ID:        component.ID,
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(installedRelease); err != nil {
		return nil, err
	}
	if err := inventoryInstance.StoreItem(invRelease, buf); err != nil {
		return nil, err
	}
	return installedRelease, nil
}

// Init setups a Helm config with a Kubernetes client capable of doing SSA
// and overrides any default namespace with given namespace.
func Init(
	namespace string,
	kubeConfig *rest.Config,
	client kube.Client[unstructured.Unstructured],
	fieldManager string,
) (*action.Configuration, error) {
	helmCfg := &action.Configuration{}
	voidLog := func(string, ...interface{}) {}
	getter := &kube.InMemoryRESTClientGetter{
		Cfg:        kubeConfig,
		RestMapper: client.RESTMapper(),
	}
	err := helmCfg.Init(getter, namespace, "secret", voidLog)
	if err != nil {
		return nil, err
	}
	helmKubeClient := helmCfg.KubeClient.(*helmKube.Client)
	// Set namespace to the release namespace in order to avoid taking the namespace from the kube config.
	helmKubeClient.Namespace = namespace
	helmCfg.KubeClient = &kube.HelmClient{
		Client:        helmKubeClient,
		DynamicClient: client,
		FieldManager:  fieldManager,
	}
	return helmCfg, nil
}

func (c *ChartReconciler) installOrUpgrade(
	ctx context.Context,
	component *ReleaseComponent,
	inventoryInstance *inventory.Instance,
) (*Release, error) {
	desiredRelease := component.Content

	log := ctx.Value(logKey{}).(*logr.Logger)

	log.Info("Loading chart")

	helmConfig := ctx.Value(configKey{}).(*action.Configuration)
	chrt, err := c.load(ctx, desiredRelease.Chart)
	if err != nil {
		return nil, err
	}

	histClient := action.NewHistory(helmConfig)
	histClient.Max = 2
	releases, err := histClient.Run(desiredRelease.Name)
	if err != nil {
		if err != driver.ErrReleaseNotFound {
			return nil, err
		}
		return c.install(ctx, desiredRelease, chrt)
	}
	if len(releases) == 1 {
		if releases[0].Info.Status == release.StatusPendingInstall {
			if err := reset(ctx, releases[0]); err != nil {
				return nil, err
			}
			return c.install(ctx, desiredRelease, chrt)
		}
	}

	drift, err := c.diff(
		ctx,
		component,
		chrt,
		releases,
		inventoryInstance,
	)
	if err != nil {
		return nil, err
	}

	if drift.driftType == driftTypeNone {
		log.Info("No changes")
		latestInternalRelease := releases[len(releases)-1]
		return &Release{
			Name:      latestInternalRelease.Name,
			Namespace: latestInternalRelease.Namespace,
			Chart:     desiredRelease.Chart,
			Values:    desiredRelease.Values,
			Version:   latestInternalRelease.Version,
		}, nil
	}

	logDrift(ctx, drift.driftType, drift.affectedManifest, err)

	upgrade := action.NewUpgrade(helmConfig)
	upgrade.PlainHTTP = c.PlainHTTP
	upgrade.Wait = false
	upgrade.Namespace = desiredRelease.Namespace
	upgrade.MaxHistory = 5
	if drift.driftType == driftTypeConflict {
		upgrade.Force = true
	}

	log.Info("Upgrading release")

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

type drift struct {
	driftType        driftType
	affectedManifest *unstructured.Unstructured
	cause            error
}

type driftType string

const (
	driftTypeConflict driftType = "conflict"
	driftTypeDeleted  driftType = "deleted"
	driftTypeUpdate   driftType = "update"
	driftTypeNone     driftType = "none"
)

func (c *ChartReconciler) diff(
	ctx context.Context,
	component *ReleaseComponent,
	loadedChart *chart.Chart,
	releases []*release.Release,
	inventoryInstance *inventory.Instance,
) (*drift, error) {
	releaseDeclaration := component.Content

	helmConfig := ctx.Value(configKey{}).(*action.Configuration)
	upgrade := action.NewUpgrade(helmConfig)
	upgrade.PlainHTTP = c.PlainHTTP
	upgrade.Wait = false
	upgrade.Namespace = releaseDeclaration.Namespace
	upgrade.DryRun = true

	release, err := upgrade.Run(releaseDeclaration.Name, loadedChart, releaseDeclaration.Values)
	if err != nil {
		release := releases[len(releases)-1]
		if !release.Info.Status.IsPending() {
			return nil, err
		}

		if err := reset(ctx, release); err != nil {
			return nil, err
		}

		return &drift{
			driftType: driftTypeUpdate,
			cause:     err,
		}, nil
	}

	decoder := yaml.NewDecoder(bytes.NewBufferString(release.Manifest))
	for {
		var unstr map[string]interface{}
		if err := decoder.Decode(&unstr); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if len(unstr) == 0 {
			continue
		}

		newManifest := &unstructured.Unstructured{Object: unstr}
		// manifests with no namespace are set to the release namespace on installation/upgrade.
		// Kube client checks whether the manifest is namespaced or not,
		// so we dont care if we set it on non namespaced manifests.
		if newManifest.GetNamespace() == "" {
			newManifest.SetNamespace(releaseDeclaration.Namespace)
		}

		obj, err := c.Client.Get(ctx, newManifest)
		if err != nil {
			switch k8sErrors.ReasonForError(err) {
			case v1.StatusReasonNotFound:
				return &drift{
					driftType:        driftTypeDeleted,
					affectedManifest: newManifest,
					cause:            err,
				}, nil
			}
			return nil, err
		}
		if obj == nil {
			return &drift{
				driftType:        driftTypeDeleted,
				affectedManifest: newManifest,
			}, nil
		}

		if err := c.Client.Apply(ctx, newManifest, c.FieldManager, kube.DryRun(true)); err != nil {
			switch k8sErrors.ReasonForError(err) {
			case v1.StatusReasonUnknown:
				return nil, err
			}

			return &drift{
				driftType:        driftTypeConflict,
				affectedManifest: newManifest,
				cause:            err,
			}, nil
		}
	}

	contentReader, err := inventoryInstance.GetItem(
		&inventory.HelmReleaseItem{
			Name:      releaseDeclaration.Name,
			Namespace: releaseDeclaration.Namespace,
			ID:        component.ID,
		},
	)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &drift{
				driftType: driftTypeDeleted,
				cause:     err,
			}, nil
		}
		return nil, err
	}
	defer contentReader.Close()

	storedRelease := Release{}
	if err := json.NewDecoder(contentReader).Decode(&storedRelease); err != nil {
		return nil, err
	}

	if isEqual := cmp.Equal(releaseDeclaration, ReleaseDeclaration{
		Name:      storedRelease.Name,
		Namespace: storedRelease.Namespace,
		Chart:     storedRelease.Chart,
		Values:    storedRelease.Values,
	}); isEqual {
		return &drift{
			driftType: driftTypeNone,
		}, nil
	}

	return &drift{
		driftType: driftTypeUpdate,
	}, nil
}

func logDrift(
	ctx context.Context,
	driftType driftType,
	newManifest *unstructured.Unstructured,
	err error,
) {
	log := *ctx.Value(logKey{}).(*logr.Logger)
	log = log.WithValues(
		"driftType",
		driftType,
	)

	if newManifest != nil {
		log = log.WithValues(
			"manifest",
			newManifest.GetName(),
			"kind",
			newManifest.GetKind(),
			"apiVersion",
			newManifest.GetAPIVersion(),
		)
	}
	if err != nil {
		log = log.WithValues(
			"err",
			err,
		)
	}

	log.V(1).Info("Drift detected")
}

func (c *ChartReconciler) install(
	ctx context.Context,
	desiredRelease ReleaseDeclaration,
	loadedChart *chart.Chart,
) (*Release, error) {
	log := ctx.Value(logKey{}).(*logr.Logger)

	helmConfig := ctx.Value(configKey{}).(*action.Configuration)
	install := action.NewInstall(helmConfig)
	install.PlainHTTP = c.PlainHTTP
	install.Wait = false
	install.ReleaseName = desiredRelease.Name
	install.CreateNamespace = true
	install.Namespace = desiredRelease.Namespace

	log.Info("Installing chart")

	release, err := install.Run(loadedChart, desiredRelease.Values)
	if err != nil {
		log.Error(err, "Installing chart failed")
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

func reset(
	ctx context.Context,
	release *release.Release,
) error {
	log := ctx.Value(logKey{}).(*logr.Logger)

	log.Info("Resetting dangling release")

	helmConfig := ctx.Value(configKey{}).(*action.Configuration)
	_, err := helmConfig.Releases.Delete(release.Name, release.Version)
	if err != nil {
		log.Error(err, "Resetting dangling release failed")
		return err
	}

	return nil
}

func (c *ChartReconciler) load(
	ctx context.Context,
	chartRequest Chart,
) (*chart.Chart, error) {
	log := ctx.Value(logKey{}).(*logr.Logger)

	var err error
	archivePath := newArchivePath(chartRequest)
	chart, err := loader.Load(archivePath.fullPath)
	if err != nil {
		pathErr := &fs.PathError{}
		if errors.As(err, &pathErr) {
			log.Info("Pulling chart")
			if err := c.pull(ctx, chartRequest, archivePath.dir); err != nil {
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

func (c *ChartReconciler) pull(
	ctx context.Context,
	chartRequest Chart,
	chartDestPath string,
) error {
	helmConfig := ctx.Value(configKey{}).(*action.Configuration)
	pull := action.NewPullWithOpts(action.WithConfig(helmConfig))
	pull.DestDir = chartDestPath

	httpClient := http.DefaultClient
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.InsecureSkipTLSverify,
		},
	}
	pull.PlainHTTP = c.PlainHTTP
	pull.InsecureSkipTLSverify = c.InsecureSkipTLSverify

	var chartRef string
	if registry.IsOCI(chartRequest.RepoURL) {
		opts := []registry.ClientOption{
			registry.ClientOptDebug(false),
			registry.ClientOptEnableCache(true),
			registry.ClientOptWriter(os.Stderr),
			registry.ClientOptHTTPClient(httpClient),
		}
		if c.PlainHTTP {
			opts = append(opts, registry.ClientOptPlainHTTP())
		}
		registryClient, err := registry.NewClient(opts...)
		if err != nil {
			return err
		}
		pull.SetRegistryClient(registryClient)

		if chartRequest.Auth != nil {
			host, _ := strings.CutPrefix(chartRequest.RepoURL, "oci://")

			var creds *cloud.Credentials
			if chartRequest.Auth.WorkloadIdentity != nil {
				provider := cloud.GetProvider(
					cloud.ProviderID(chartRequest.Auth.WorkloadIdentity.Provider),
					host,
					httpClient,
				)
				creds, err = provider.FetchCredentials(ctx)
				if err != nil {
					return err
				}
			} else {
				creds, err = c.readCredentialsFromSecret(ctx, chartRequest)
				if err != nil {
					return err
				}
			}

			if err := registryClient.Login(
				host,
				registry.LoginOptBasicAuth(creds.Username, creds.Password),
			); err != nil {
				return err
			}
		}

		chartRef = fmt.Sprintf("%s/%s", chartRequest.RepoURL, chartRequest.Name)
	} else {
		if chartRequest.Auth != nil {
			creds, err := c.readCredentialsFromSecret(ctx, chartRequest)
			if err != nil {
				return err
			}
			pull.Username = creds.Username
			pull.Password = creds.Password
		}

		pull.RepoURL = chartRequest.RepoURL
		chartRef = chartRequest.Name
	}

	pull.Settings = cli.New()
	pull.Version = chartRequest.Version
	err := os.MkdirAll(chartDestPath, 0700)
	if err != nil {
		return err
	}

	_, err = pull.Run(chartRef)
	if err != nil {
		return err
	}
	return nil
}

func (c *ChartReconciler) readCredentialsFromSecret(
	ctx context.Context,
	chartRequest Chart,
) (*cloud.Credentials, error) {
	if chartRequest.Auth.SecretRef == nil {
		return nil, fmt.Errorf("%w: secretRef not set", ErrAuthSecretValueNotFound)
	}

	secretReq := &unstructured.Unstructured{}
	secretReq.SetKind("Secret")
	secretReq.SetAPIVersion("v1")
	secretReq.SetName(chartRequest.Auth.SecretRef.Name)
	secretReq.SetNamespace(chartRequest.Auth.SecretRef.Namespace)
	secret, err := c.Client.Get(ctx, secretReq)
	if err != nil {
		return nil, err
	}

	data, found := secret.Object["data"].(map[string]interface{})
	var username, password string
	if found {
		username, err = getSecretValue(data, "username", false)
		if err != nil {
			return nil, err
		}
		password, err = getSecretValue(data, "password", false)
		if err != nil {
			return nil, err
		}
	} else {
		stringData, found := secret.Object["stringData"].(map[string]string)
		if !found {
			return nil, err
		}
		username = stringData["username"]
		password = stringData["password"]
	}

	return &cloud.Credentials{
		Username: username,
		Password: password,
	}, nil
}

func getSecretValue(data map[string]interface{}, key string, isOptional bool) (string, error) {
	value := data[key]
	if value == nil {
		if isOptional {
			return "", nil
		}
		return "", fmt.Errorf("%w: %s is empty", ErrAuthSecretValueNotFound, key)
	}
	bytes, err := base64.StdEncoding.DecodeString(value.(string))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
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
