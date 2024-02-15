package inventory

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// ErrWrongInventoryKey occurs when a stored object has been read,
	// which doesn't follow the expected format.
	// This can only happen through an incompatible change, like editing the inventory directly.
	ErrWrongInventoryKey = errors.New("Inventory key is incorrect")
)

// Component represents a stored Declcd component with its id and items.
// It is a component which is part of the current cluster state.
type Component struct {
	id    string
	items map[string]Item
}

// Items returns the metadata to all stored objects of this component.
func (component Component) Items() map[string]Item {
	return component.items
}

// Item is a small representation of a stored object.
type Item interface {
	TypeMeta() *v1.TypeMeta
	ComponentID() string
	Name() string
	Namespace() string
	AsKey() string
}

// HelmReleaseItem is a small inventory representation of a Release.
// Release is a running instance of a Chart.
// When a chart is installed, the ChartReconciler creates a release to track that installation.
type HelmReleaseItem struct {
	componentID string
	name        string
	namespace   string
}

var _ Item = (*HelmReleaseItem)(nil)

// NewHelmReleaseItem constructs a ReleaseMetadata,
// which is a small representation of a Release.
func NewHelmReleaseItem(componentID string, name string, namespace string) HelmReleaseItem {
	return HelmReleaseItem{
		componentID: componentID,
		name:        name,
		namespace:   namespace,
	}
}

// Nil for a helm release.
func (hr HelmReleaseItem) TypeMeta() *v1.TypeMeta {
	return nil
}

// Name of the helm release.
func (hr HelmReleaseItem) Name() string {
	return hr.name
}

// Namespace of the helm release.
func (hr HelmReleaseItem) Namespace() string {
	return hr.namespace
}

// ComponentID is a link to the component this release belongs to.
func (hr HelmReleaseItem) ComponentID() string {
	return hr.componentID
}

// AsKey returns the string representation of the release.
// This is used as an identifier in the inventory.
func (hr HelmReleaseItem) AsKey() string {
	return fmt.Sprintf("%s_%s_%s_%s", hr.componentID, hr.name, hr.namespace, "HelmRelease")
}

// ManifestItem a small inventory representation of a ManifestItem.
// ManifestItem is a Kubernetes object.
type ManifestItem struct {
	typeMeta    v1.TypeMeta
	componentID string
	name        string
	namespace   string
}

var _ Item = (*ManifestItem)(nil)

// NewManifestItem constructs a manifest,
// which is a small representation of a Manifest.
func NewManifestItem(
	typeMeta v1.TypeMeta,
	componentID string,
	name string,
	namespace string,
) ManifestItem {
	return ManifestItem{
		typeMeta:    typeMeta,
		componentID: componentID,
		name:        name,
		namespace:   namespace,
	}
}

// TypeMeta describes an individual object.
func (manifest ManifestItem) TypeMeta() *v1.TypeMeta {
	return &manifest.typeMeta
}

// Name of the manifest.
func (manifest ManifestItem) Name() string {
	return manifest.name
}

// Namespace of the manifest.
func (manifest ManifestItem) Namespace() string {
	return manifest.namespace
}

// ComponentID is a link to the component this manifest belongs to.
func (manifest ManifestItem) ComponentID() string {
	return manifest.componentID
}

// AsKey returns the string representation of the manifest.
// This is used as an identifier in the inventory.
func (manifest ManifestItem) AsKey() string {
	group := ""
	version := ""
	groupVersion := strings.Split(manifest.typeMeta.APIVersion, "/")
	if len(groupVersion) == 1 {
		version = groupVersion[0]
	} else {
		group = groupVersion[0]
		version = groupVersion[1]
	}
	return fmt.Sprintf(
		"%s_%s_%s_%s_%s_%s",
		manifest.componentID,
		manifest.name,
		manifest.namespace,
		manifest.typeMeta.Kind,
		group,
		version,
	)
}

// Storage represents all stored Declcd components.
// It is effectively the current cluster state.
type Storage struct {
	components map[string]Component
}

// Components returns all stored Declcd components.
func (inv Storage) Components() map[string]Component {
	return inv.components
}

// HasItem evaluates whether an item is part of the current cluster state.
func (inv Storage) HasItem(item Item) bool {
	exists := false
	if comp, found := inv.components[item.ComponentID()]; found {
		_, exists = comp.items[item.AsKey()]
	}
	return exists
}

// Manager is responsible for maintaining the current cluster state.
// It can store, delete and read items from the inventory.
type Manager struct {
	Log  logr.Logger
	Path string
}

// Load reads the current inventory and returns all the stored components.
func (manager *Manager) Load() (*Storage, error) {
	if err := os.MkdirAll(manager.Path, 0700); err != nil {
		return nil, err
	}
	components := make(map[string]Component)
	err := filepath.WalkDir(manager.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			key := d.Name()
			identifier := strings.Split(key, "_")
			componentID := identifier[0]
			comp, found := components[componentID]
			if !found {
				comp = Component{
					id:    componentID,
					items: make(map[string]Item),
				}
			}
			if len(identifier) == 4 {
				kind := identifier[3]
				if kind != "HelmRelease" {
					return fmt.Errorf(
						"%w: key with only 4 identifiers is expected to be a HelmRelease",
						ErrWrongInventoryKey,
					)
				}
				comp.items[key] = NewHelmReleaseItem(
					componentID,
					identifier[1],
					identifier[2],
				)
			} else {
				if len(identifier) != 6 {
					return fmt.Errorf("%w: key '%s' does not contain 6 identifiers", ErrWrongInventoryKey, key)
				}
				group := identifier[4]
				version := identifier[5]
				apiVersion := ""
				if group == "" {
					apiVersion = version
				} else {
					apiVersion = fmt.Sprintf("%s/%s", group, version)
				}
				comp.items[key] = NewManifestItem(
					v1.TypeMeta{
						Kind:       identifier[3],
						APIVersion: apiVersion,
					},
					componentID,
					identifier[1],
					identifier[2],
				)
			}
			components[componentID] = comp
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Storage{
		components: components,
	}, nil
}

// GetItem opens the item file for reading.
// If there is an error, it will be of type *PathError.
func (manager Manager) GetItem(item Item) (io.ReadCloser, error) {
	itemFile, err := os.Open(filepath.Join(manager.Path, item.ComponentID(), item.AsKey()))
	if err != nil {
		return nil, err
	}
	return itemFile, nil
}

// StoreItem persists given item with optional content in the inventory.
func (manager Manager) StoreItem(item Item, contentReader io.Reader) error {
	dir := filepath.Join(manager.Path, item.ComponentID())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(dir, item.AsKey()))
	if err != nil {
		return err
	}
	defer file.Close()
	if contentReader != nil {
		bufferedReadWriter := bufio.NewReadWriter(
			bufio.NewReader(contentReader),
			bufio.NewWriter(file),
		)
		for {
			line, err := bufferedReadWriter.ReadString('\n')
			if err == io.EOF {
				break
			}
			_, err = bufferedReadWriter.WriteString(line)
			if err != nil {
				return err
			}
		}
		if err = bufferedReadWriter.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// DeleteItem removes the item from the inventory.
// Declcd will not be tracking its current state anymore.
func (manager Manager) DeleteItem(item Item) error {
	dir := filepath.Join(manager.Path, item.ComponentID())
	dirFile, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirFile.Close()
	_, err = dirFile.Readdirnames(1)
	if err == io.EOF {
		if err := os.Remove(dir); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, item.AsKey()))
}
