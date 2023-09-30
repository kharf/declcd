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

type HelmRelease struct {
	Name      string
	Namespace string
}

func (hr HelmRelease) AsKey() string {
	return fmt.Sprintf("%s_%s_%s", hr.Name, hr.Namespace, "HelmRelease")
}

type Storage struct {
	Manifests    map[string]Manifest
	HelmReleases map[string]HelmRelease
}

func (inv Storage) HasManifest(manifest Manifest) bool {
	_, exists := inv.Manifests[manifest.AsKey()]
	return exists
}

func (inv Storage) HasRelease(release HelmRelease) bool {
	_, exists := inv.HelmReleases[release.AsKey()]
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
	releases := make(map[string]HelmRelease)
	for _, file := range files {
		key := file.Name()
		identifier := strings.Split(key, "_")
		if len(identifier) == 3 {
			kind := identifier[2]
			if kind != "HelmRelease" {
				return nil, fmt.Errorf("%w: key with only 3 identifiers is expected to be a HelmRelease", ErrWrongInventoryKey)
			}

			releases[key] = HelmRelease{
				Name:      identifier[0],
				Namespace: identifier[1],
			}
		} else {
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

			manifests[key] = Manifest{
				TypeMeta: v1.TypeMeta{
					Kind:       identifier[2],
					APIVersion: apiVersion,
				},
				Name:      identifier[0],
				Namespace: identifier[1],
			}
		}
	}

	return &Storage{
		Manifests:    manifests,
		HelmReleases: releases,
	}, nil
}

func (manager Manager) StoreManifest(inventoryManifest Manifest) error {
	return os.WriteFile(filepath.Join(manager.Path, inventoryManifest.AsKey()), []byte{}, 0700)
}

func (manager Manager) StoreHelmRelease(inventoryHelmRelease HelmRelease) error {
	return os.WriteFile(filepath.Join(manager.Path, inventoryHelmRelease.AsKey()), []byte{}, 0700)
}

func (manager Manager) DeleteManifest(inventoryManifest Manifest) error {
	return os.Remove(filepath.Join(manager.Path, inventoryManifest.AsKey()))
}

func (manager Manager) DeleteHelmRelease(inventoryHelmRelease HelmRelease) error {
	return os.Remove(filepath.Join(manager.Path, inventoryHelmRelease.AsKey()))
}
