package inventory

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/kharf/declcd/pkg/component"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrWrongInventoryKey = errors.New("Inventory key is incorrect")
)

type Component struct {
	id           string
	manifests    map[string]component.ManifestMetadata
	helmReleases map[string]component.HelmReleaseMetadata
}

func (component Component) Manifests() map[string]component.ManifestMetadata {
	return component.manifests
}

func (component Component) HelmReleases() map[string]component.HelmReleaseMetadata {
	return component.helmReleases
}

type Storage struct {
	components map[string]Component
}

func NewStorage(components map[string]Component) Storage {
	return Storage{
		components: components,
	}
}

func (inv Storage) Components() map[string]Component {
	return inv.components
}

func (inv Storage) HasManifest(manifest component.ManifestMetadata) bool {
	exists := false
	if comp, found := inv.components[manifest.ComponentID()]; found {
		_, exists = comp.manifests[manifest.AsKey()]
	}
	return exists
}

func (inv Storage) HasRelease(release component.HelmReleaseMetadata) bool {
	exists := false
	if comp, found := inv.components[release.ComponentID()]; found {
		_, exists = comp.helmReleases[release.AsKey()]
	}
	return exists
}

type Manager struct {
	Log  logr.Logger
	Path string
}

func (manager Manager) Load() (*Storage, error) {
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
					id:           componentID,
					manifests:    make(map[string]component.ManifestMetadata),
					helmReleases: make(map[string]component.HelmReleaseMetadata),
				}
			}
			if len(identifier) == 4 {
				kind := identifier[3]
				if kind != "HelmRelease" {
					return fmt.Errorf("%w: key with only 4 identifiers is expected to be a HelmRelease", ErrWrongInventoryKey)
				}
				comp.helmReleases[key] = component.NewHelmReleaseMetadata(
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
				comp.manifests[key] = component.NewManifestMetadata(
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

func (manager Manager) StoreManifest(inventoryManifest component.ManifestMetadata) error {
	dir := filepath.Join(manager.Path, inventoryManifest.ComponentID())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, inventoryManifest.AsKey()), []byte{}, 0700)
}

func (manager Manager) StoreHelmRelease(inventoryHelmRelease component.HelmReleaseMetadata) error {
	dir := filepath.Join(manager.Path, inventoryHelmRelease.ComponentID())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, inventoryHelmRelease.AsKey()), []byte{}, 0700)
}

func (manager Manager) DeleteManifest(inventoryManifest component.ManifestMetadata) error {
	dir := filepath.Join(manager.Path, inventoryManifest.ComponentID())
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
	return os.Remove(filepath.Join(dir, inventoryManifest.AsKey()))
}

func (manager Manager) DeleteHelmRelease(inventoryHelmRelease component.HelmReleaseMetadata) error {
	dir := filepath.Join(manager.Path, inventoryHelmRelease.ComponentID())
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
	return os.Remove(filepath.Join(dir, inventoryHelmRelease.AsKey()))
}
