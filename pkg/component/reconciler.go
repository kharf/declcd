package component

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/inventory"
	"github.com/kharf/declcd/pkg/kube"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Reconciler reads Components with their desired state
// and applies them on a Kubernetes cluster.
// It stores objects in the inventory.
type Reconciler struct {
	Log logr.Logger

	// DynamicClient connects to a Kubernetes cluster
	// to create, read, update and delete manifests/objects.
	DynamicClient kube.Client[unstructured.Unstructured]

	// ChartReconciler reads Helm Packages with their desired state
	// and applies them on a Kubernetes cluster.
	// It stores releases in the inventory, but never collects it.
	ChartReconciler helm.ChartReconciler

	// Instance is a representation of an inventory.
	// It can store, delete and read items.
	// The object does not include the storage itself, it only holds a reference to the storage.
	InventoryInstance *inventory.Instance

	// Managers identify distinct workflows that are modifying the object (especially useful on conflicts!),
	FieldManager string
}

func (reconciler *Reconciler) Reconcile(
	ctx context.Context,
	instance Instance,
) error {
	switch componentInstance := instance.(type) {
	case *Manifest:
		reconciler.Log.Info(
			"Applying manifest",
			"namespace",
			componentInstance.Content.GetNamespace(),
			"name",
			componentInstance.Content.GetName(),
			"kind",
			componentInstance.Content.GetKind(),
		)

		if err := reconciler.DynamicClient.Apply(ctx, &componentInstance.Content, reconciler.FieldManager, kube.Force(true)); err != nil {
			return err
		}

		invManifest := &inventory.ManifestItem{
			ID: componentInstance.ID,
			TypeMeta: v1.TypeMeta{
				Kind:       componentInstance.Content.GetKind(),
				APIVersion: componentInstance.Content.GetAPIVersion(),
			},
			Name:      componentInstance.Content.GetName(),
			Namespace: componentInstance.Content.GetNamespace(),
		}

		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(componentInstance.Content.Object); err != nil {
			return err
		}

		if err := reconciler.InventoryInstance.StoreItem(invManifest, buf); err != nil {
			return err
		}

	case *helm.ReleaseComponent:
		if _, err := reconciler.ChartReconciler.Reconcile(
			ctx,
			componentInstance,
		); err != nil {
			return err
		}
	}
	return nil
}
