package garbage

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
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
	inventory, err := c.InventoryManager.Load()
	if err != nil {
		return err
	}
	for componentID, invComponent := range inventory.Components() {
		if node := dag.Get(componentID); node != nil {
			for _, invManifest := range invComponent.Manifests() {
				collect := true
				for _, targetManifest := range node.Manifests() {
					if compareManifest(invManifest, targetManifest) {
						collect = false
						break
					}
				}
				if collect {
					if err := c.collectManifest(ctx, invManifest); err != nil {
						return err
					}
				}
			}
			for _, invHr := range invComponent.HelmReleases() {
				collect := true
				for _, hr := range node.HelmReleases() {
					if compareHelmRelease(invHr, hr) {
						collect = false
						break
					}
				}
				if collect {
					if err := c.collectHelmRelease(invHr); err != nil {
						return err
					}
				}
			}
		} else {
			for _, invManifest := range invComponent.Manifests() {
				if err := c.collectManifest(ctx, invManifest); err != nil {
					return err
				}
			}
			for _, invHr := range invComponent.HelmReleases() {
				if err := c.collectHelmRelease(invHr); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c Collector) collectHelmRelease(invHr component.HelmReleaseMetadata) error {
	c.Log.Info("Collecting unreferenced helm release", "component", invHr.ComponentID(), "namespace", invHr.Namespace(), "name", invHr.Name())
	client := action.NewUninstall(&c.HelmConfig)
	client.Wait = false
	_, err := client.Run(invHr.Name())
	if err != nil {
		return err
	}
	if err := c.InventoryManager.DeleteHelmRelease(invHr); err != nil {
		return err
	}
	return nil
}

func (c Collector) collectManifest(ctx context.Context, invManifest component.ManifestMetadata) error {
	c.Log.Info("Collecting unreferenced manifest", "component", invManifest.ComponentID(), "namespace", invManifest.Namespace(), "name", invManifest.Name(), "kind", invManifest.Kind)
	unstr := &unstructured.Unstructured{}
	unstr.SetName(invManifest.Name())
	unstr.SetNamespace(invManifest.Namespace())
	unstr.SetKind(invManifest.Kind)
	unstr.SetAPIVersion(invManifest.APIVersion)
	if err := c.Client.Delete(ctx, unstr); err != nil {
		return err
	}
	if err := c.InventoryManager.DeleteManifest(invManifest); err != nil {
		return err
	}
	return nil
}

func compareManifest(inventoryManifest component.ManifestMetadata, manifest component.ManifestMetadata) bool {
	return inventoryManifest.ComponentID() == manifest.ComponentID() &&
		inventoryManifest.Kind == manifest.Kind &&
		inventoryManifest.APIVersion == manifest.APIVersion &&
		inventoryManifest.Namespace() == manifest.Namespace() &&
		inventoryManifest.Name() == manifest.Name()
}

func compareHelmRelease(inventoryRelease component.HelmReleaseMetadata, release component.HelmReleaseMetadata) bool {
	return inventoryRelease.ComponentID() == release.ComponentID() &&
		inventoryRelease.Name() == release.Name() &&
		inventoryRelease.Namespace() == release.Namespace()
}
