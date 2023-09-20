package garbage

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Collector struct {
	Log              logr.Logger
	Client           *kube.Client
	InventoryManager inventory.Manager
}

func (c Collector) Collect(ctx context.Context, inventory inventory.Storage, renderedManifests []unstructured.Unstructured) error {
	for _, invManifest := range inventory.Manifests {
		collect := true
		for _, targetManifest := range renderedManifests {
			if compare(invManifest, targetManifest) {
				collect = false
				break
			}
		}

		if collect {
			c.Log.Info("collecting unreferenced object", "namespace", invManifest.Namespace, "name", invManifest.Name, "kind", invManifest.Kind)
			unstr := &unstructured.Unstructured{}
			unstr.SetName(invManifest.Name)
			unstr.SetNamespace(invManifest.Namespace)
			unstr.SetKind(invManifest.Kind)
			unstr.SetAPIVersion(invManifest.APIVersion)
			if err := c.Client.Delete(ctx, unstr); err != nil {
				return err
			}
			if err := c.InventoryManager.Delete(invManifest); err != nil {
				return err
			}
		}
	}
	return nil
}

func compare(inventoryManifest inventory.Manifest, manifest unstructured.Unstructured) bool {
	return inventoryManifest.Kind == manifest.GetKind() &&
		inventoryManifest.APIVersion == manifest.GetAPIVersion() &&
		inventoryManifest.Namespace == manifest.GetNamespace() &&
		inventoryManifest.Name == manifest.GetName()
}
