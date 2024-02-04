package garbage

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Collector struct {
	Log              logr.Logger
	Client           *kube.DynamicClient
	InventoryManager inventory.Manager
	HelmConfig       action.Configuration
}

func (c Collector) Collect(ctx context.Context, dag component.DependencyGraph) error {
	storage, err := c.InventoryManager.Load()
	if err != nil {
		return err
	}
	for componentID, invComponent := range storage.Components() {
		if node := dag.Get(componentID); node != nil {
			for _, item := range invComponent.Items() {
				collect := true
				switch item.(type) {
				case inventory.HelmReleaseItem:
					for _, hr := range node.HelmReleases() {
						if compareHelmRelease(item.(inventory.HelmReleaseItem), hr) {
							collect = false
							break
						}
					}
					if collect {
						if err := c.collectHelmRelease(item.(inventory.HelmReleaseItem)); err != nil {
							return err
						}
					}
				case inventory.Manifest:
					for _, manifest := range node.Manifests() {
						if compareManifest(item.(inventory.Manifest), manifest) {
							collect = false
							break
						}
					}
					if collect {
						if err := c.collectManifest(ctx, item.(inventory.Manifest)); err != nil {
							return err
						}
					}
				}
			}
		} else {
			for _, item := range invComponent.Items() {
				switch item.(type) {
				case inventory.HelmReleaseItem:
					if err := c.collectHelmRelease(item.(inventory.HelmReleaseItem)); err != nil {
						return err
					}
				case inventory.Manifest:
					if err := c.collectManifest(ctx, item.(inventory.Manifest)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c Collector) collectHelmRelease(invHr inventory.HelmReleaseItem) error {
	c.Log.Info("Collecting unreferenced helm release", "component", invHr.ComponentID(), "namespace", invHr.Namespace(), "name", invHr.Name())
	client := action.NewUninstall(&c.HelmConfig)
	client.Wait = false
	_, err := client.Run(invHr.Name())
	if err != nil {
		return err
	}
	if err := c.InventoryManager.DeleteItem(invHr); err != nil {
		return err
	}
	return nil
}

func (c Collector) collectManifest(ctx context.Context, invManifest inventory.Manifest) error {
	c.Log.Info("Collecting unreferenced manifest", "component", invManifest.ComponentID(), "namespace", invManifest.Namespace(), "name", invManifest.Name(), "kind", invManifest.TypeMeta().Kind)
	unstr := &unstructured.Unstructured{}
	unstr.SetName(invManifest.Name())
	unstr.SetNamespace(invManifest.Namespace())
	unstr.SetKind(invManifest.TypeMeta().Kind)
	unstr.SetAPIVersion(invManifest.TypeMeta().APIVersion)
	if err := c.Client.Delete(ctx, unstr); err != nil {
		return err
	}
	if err := c.InventoryManager.DeleteItem(invManifest); err != nil {
		return err
	}
	return nil
}

func compareManifest(inventoryManifest inventory.Manifest, manifest kube.ManifestMetadata) bool {
	return inventoryManifest.ComponentID() == manifest.ComponentID() &&
		inventoryManifest.TypeMeta().Kind == manifest.Kind &&
		inventoryManifest.TypeMeta().APIVersion == manifest.APIVersion &&
		inventoryManifest.Namespace() == manifest.Namespace() &&
		inventoryManifest.Name() == manifest.Name()
}

func compareHelmRelease(inventoryRelease inventory.HelmReleaseItem, release helm.ReleaseMetadata) bool {
	return inventoryRelease.ComponentID() == release.ComponentID() &&
		inventoryRelease.Name() == release.Name() &&
		inventoryRelease.Namespace() == release.Namespace()
}
