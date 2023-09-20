package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrWrongInventoryKey = errors.New("inventory key is incorrect")
)

type Manifest struct {
	v1.TypeMeta
	Name      string
	Namespace string
}

func (manifest Manifest) AsKey() string {
	group := ""
	version := ""
	groupVersion := strings.Split(manifest.APIVersion, "/")
	if len(groupVersion) == 1 {
		version = groupVersion[0]
	} else {
		group = groupVersion[0]
		version = groupVersion[1]
	}

	return fmt.Sprintf("%s_%s_%s_%s_%s", manifest.Name, manifest.Namespace, manifest.Kind, group, version)
}

type Storage struct {
	Manifests map[string]Manifest
}

func (inv Storage) Has(manifest Manifest) bool {
	_, exists := inv.Manifests[manifest.AsKey()]
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

	files, err := os.ReadDir(manager.Path)
	if err != nil {
		return nil, err
	}

	manifests := make(map[string]Manifest)
	for _, file := range files {
		key := file.Name()
		identifier := strings.Split(key, "_")
		if len(identifier) != 5 {
			return nil, fmt.Errorf("%w: key does not contain 5 identifiers", ErrWrongInventoryKey)
		}

		group := identifier[3]
		version := identifier[4]
		apiVersion := ""
		if group == "" {
			apiVersion = version
		} else {
			apiVersion = fmt.Sprintf("%s/%s", group, version)
		}

		manifest := Manifest{
			TypeMeta: v1.TypeMeta{
				Kind:       identifier[2],
				APIVersion: apiVersion,
			},
			Name:      identifier[0],
			Namespace: identifier[1],
		}
		manifests[key] = manifest
	}

	return &Storage{
		Manifests: manifests,
	}, nil
}

func (manager Manager) Store(inventoryManifest Manifest) error {
	return os.WriteFile(filepath.Join(manager.Path, inventoryManifest.AsKey()), []byte{}, 0700)
}

func (manager Manager) Delete(inventoryManifest Manifest) error {
	return os.Remove(filepath.Join(manager.Path, inventoryManifest.AsKey()))
}
