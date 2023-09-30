package garbage

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Collector struct {
	Log              logr.Logger
	Client           *kube.Client
	InventoryManager inventory.Manager
	HelmConfig       action.Configuration
}

func (c Collector) Collect(ctx context.Context, renderedManifests []unstructured.Unstructured, releases []helm.Release) error {
	inventory, err := c.InventoryManager.Load()
	if err != nil {
		return err
	}

	for _, invManifest := range inventory.Manifests {
		collect := true
		for _, targetManifest := range renderedManifests {
			if compareManifest(invManifest, targetManifest) {
				collect = false
				break
			}
		}

		if collect {
			c.Log.Info("collecting unreferenced manifest", "namespace", invManifest.Namespace, "name", invManifest.Name, "kind", invManifest.Kind)
			unstr := &unstructured.Unstructured{}
			unstr.SetName(invManifest.Name)
			unstr.SetNamespace(invManifest.Namespace)
			unstr.SetKind(invManifest.Kind)
			unstr.SetAPIVersion(invManifest.APIVersion)
			if err := c.Client.Delete(ctx, unstr); err != nil {
				return err
			}
			if err := c.InventoryManager.DeleteManifest(invManifest); err != nil {
				return err
			}
		}
	}

	for _, invHr := range inventory.HelmReleases {
		collect := true
		for _, hr := range releases {
			if compareHelmRelease(invHr, hr) {
				collect = false
				break
			}
		}

		if collect {
			c.Log.Info("collecting unreferenced helm release", "namespace", invHr.Namespace, "name", invHr.Name)
			client := action.NewUninstall(&c.HelmConfig)
			client.Wait = false
			_, err := client.Run(invHr.Name)
			if err != nil {
				return err
			}
			if err := c.InventoryManager.DeleteHelmRelease(invHr); err != nil {
				return err
			}
		}

	}
	return nil
}

func compareManifest(inventoryManifest inventory.Manifest, manifest unstructured.Unstructured) bool {
	return inventoryManifest.Kind == manifest.GetKind() &&
		inventoryManifest.APIVersion == manifest.GetAPIVersion() &&
		inventoryManifest.Namespace == manifest.GetNamespace() &&
		inventoryManifest.Name == manifest.GetName()
}

func compareHelmRelease(inventoryRelease inventory.HelmRelease, release helm.Release) bool {
	return inventoryRelease.Name == release.Name &&
		inventoryRelease.Namespace == release.Namespace
}
