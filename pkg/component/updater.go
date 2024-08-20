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

package component

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/kharf/declcd/internal/slices"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/vcs"
	"k8s.io/kubernetes/pkg/util/parsers"
)

var (
	ErrUnexpectedResponse = errors.New("Unexpected response")
	ErrChartNotFound      = errors.New("Chart not found")
)

// UpdateStrategy defines the container image or helm chart update strategy to calculate the latest version.
type UpdateStrategy int

const (
	// Semantic Versioning as defined in https://semver.org/.
	SemVer UpdateStrategy = iota
)

// ContainerUpdateTarget defines the container image to be updated.
type ContainerUpdateTarget struct {
	// Image value of the 'tagged' field.
	// It has the format 'repository:tag@digest'.
	Image string

	// Reference to the struct holding repository and version fields.
	UnstructuredNode map[string]any

	// Field key or label of the version field.
	UnstructuredKey string
}

func (c *ContainerUpdateTarget) Name() string {
	name, _, _ := strings.Cut(c.Image, ":")
	return name
}

func (c *ContainerUpdateTarget) Parse() (string, string, string, error) {
	return parsers.ParseImageName(c.Image)
}

func (c *ContainerUpdateTarget) GetStructValue() string {
	return c.UnstructuredNode[c.UnstructuredKey].(string)
}

func (c *ContainerUpdateTarget) SetStructValue(newValue string) {
	c.UnstructuredNode[c.UnstructuredKey] = newValue
}

var _ UpdateTarget = (*ContainerUpdateTarget)(nil)

// ChartUpdateTarget defines the helm chart to be updated.
type ChartUpdateTarget struct {
	Chart *helm.Chart
}

func (c *ChartUpdateTarget) Name() string {
	return c.Chart.Name
}

func (c *ChartUpdateTarget) Parse() (string, string, string, error) {
	return c.Chart.RepoURL, c.Chart.Version, "", nil
}

func (c *ChartUpdateTarget) GetStructValue() string {
	return c.Chart.Version
}

func (c *ChartUpdateTarget) SetStructValue(newValue string) {
	c.Chart.Version = newValue
}

var _ UpdateTarget = (*ChartUpdateTarget)(nil)

type UpdateTarget interface {
	Name() string
	Parse() (string, string, string, error)
	SetStructValue(newValue string)
	GetStructValue() string
}

// UpdateInstruction is an instruction to tell Declcd to automatically update container images or helm charts.
type UpdateInstruction struct {
	Strategy   UpdateStrategy
	Constraint string
	SecretRef  string

	// File path of the 'tagged' field.
	File string

	// Line number of the field holding the version value.
	Line int

	// Object to be updated.
	// It can be either a container image or a helm chart.
	// A container image has the format 'repository:tag@digest'.
	// A helm repository has the format 'oci://repository' or 'https://repository'.
	Target UpdateTarget
}

// Update represents the result of an update operation.
type Update struct {
	// CommitHash contains the SHA1 of the commit.
	CommitHash string

	// NewVersion contains the updated version.
	NewVersion string
}

// Updater accepts update instructions that tell which images to update.
// For every instruction it contacts image registries to fetch remote tags and calculates the latest tag based on the provided update strategy.
// If the latest tag is greater than the current tag, it updates the image and commits the changes.
// It pushes its changes to remote before returning.
type Updater struct {
	Log        logr.Logger
	Repository *vcs.Repository
}

// Update accepts update instructions that tell which images to update and returns update results.
func (updater *Updater) Update(updateInstructions []UpdateInstruction) ([]Update, error) {
	var updates []Update
	for _, updateInstr := range updateInstructions {
		strategy := getStrategy(updateInstr.Strategy, updateInstr.Constraint)

		var update *Update
		var err error
		switch target := updateInstr.Target.(type) {
		case *ContainerUpdateTarget:
			var repoName, currentVersion string
			repoName, currentVersion, _, err = updateInstr.Target.Parse()
			if err != nil {
				return nil, err
			}

			update, err = updater.updateContainerIfNotLatest(strategy, currentVersion, repoName, updateInstr)

		case *ChartUpdateTarget:
			update, err = updater.updateHelmChartIfNotLatest(strategy, target, updateInstr)
		}

		if err != nil {
			return nil, err
		}

		if update == nil {
			continue
		}

		updates = append(updates, *update)
	}

	if len(updates) > 0 {
		if err := updater.Repository.Push(); err != nil {
			return nil, err
		}
	}

	return updates, nil
}

func (updater *Updater) updateHelmChartIfNotLatest(
	strategy Strategy,
	target *ChartUpdateTarget,
	updateInstr UpdateInstruction,
) (*Update, error) {
	if registry.IsOCI(target.Chart.RepoURL) {
		host, _ := strings.CutPrefix(target.Chart.RepoURL, "oci://")
		repoName := fmt.Sprintf("%s/%s", host, target.Chart.Name)

		return updater.updateContainerIfNotLatest(
			strategy,
			target.Chart.Version,
			repoName,
			updateInstr,
		)
	}

	return updater.updateHttpHelmChartIfNotLatest(strategy, target, updateInstr)
}

func (updater *Updater) updateHttpHelmChartIfNotLatest(
	strategy Strategy,
	target *ChartUpdateTarget,
	updateInstr UpdateInstruction,
) (*Update, error) {
	resp, err := http.DefaultClient.Get(fmt.Sprintf("%s/index.yaml", target.Chart.RepoURL))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", ErrUnexpectedResponse, body)
	}

	decoder := yaml.NewDecoder(resp.Body)
	var indexFile repo.IndexFile
	if err := decoder.Decode(&indexFile); err != nil {
		return nil, err
	}

	chartVersions, found := indexFile.Entries[target.Chart.Name]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrChartNotFound, target.Chart.Name)
	}
	if len(chartVersions) == 0 {
		return nil, nil
	}

	update, hasUpdate, err := updater.updateIfNotLatest(
		target.Chart.Version,
		&helm.ChartVersionIter{Versions: chartVersions},
		updateInstr,
		strategy,
	)
	if err != nil {
		return nil, err
	}
	if !hasUpdate {
		return nil, nil
	}

	return update, nil
}

func (updater *Updater) updateContainerIfNotLatest(
	strategy Strategy,
	currentVersion string,
	repoName string,
	updateInstr UpdateInstruction,
) (*Update, error) {
	remoteVersions, err := getRemoteVersionsFromRegistry(repoName)
	if err != nil {
		return nil, err
	}

	update, hasUpdate, err := updater.updateIfNotLatest(
		currentVersion,
		&slices.Iter[string]{Slice: remoteVersions},
		updateInstr,
		strategy,
	)
	if err != nil {
		return nil, err
	}
	if !hasUpdate {
		return nil, nil
	}

	return update, nil
}

func (updater *Updater) updateIfNotLatest(
	currVersion string,
	remoteVersions VersionIter,
	updateInstr UpdateInstruction,
	strategy Strategy,
) (*Update, bool, error) {
	newVersion, hasNewVersion, err := strategy.HasNewerRemoteVersion(
		currVersion,
		remoteVersions,
	)
	if err != nil {
		return nil, false, err
	}
	if !hasNewVersion {
		return nil, false, nil
	}

	name := updateInstr.Target.Name()
	updater.Log.Info(
		"Updating",
		"target",
		name,
		"newVersion",
		newVersion,
		"file",
		updateInstr.File,
	)

	if err := updater.updateVersion(updateInstr, currVersion, newVersion); err != nil {
		return nil, false, err
	}

	hash, err := updater.Repository.Commit(updateInstr.File,
		fmt.Sprintf(
			"chore(update): bump %s to %s",
			name,
			newVersion,
		),
	)
	if err != nil {
		return nil, false, err
	}

	return &Update{
		CommitHash: hash,
		NewVersion: newVersion,
	}, true, nil
}

func getRemoteVersionsFromRegistry(repoName string) ([]string, error) {
	repository, err := name.NewRepository(repoName)
	if err != nil {
		return nil, err
	}

	remoteVersions, err := remote.List(repository)
	if err != nil {
		return nil, err
	}
	return remoteVersions, nil
}

func (updater *Updater) updateVersion(
	updateInstr UpdateInstruction,
	currVersion string,
	newVersion string,
) error {
	file, err := os.Open(updateInstr.File)
	if err != nil {
		return err
	}
	defer file.Close()

	newFile, err := os.CreateTemp("", "update-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(newFile.Name())
	}()

	scanner := bufio.NewScanner(file)
	writer := bufio.NewWriter(newFile)

	currLineNumber := 1
	for scanner.Scan() {
		var currLine string
		if currLineNumber == updateInstr.Line {
			newValue := strings.Replace(
				updateInstr.Target.GetStructValue(),
				currVersion,
				newVersion,
				1,
			)
			currLine = strings.Replace(
				scanner.Text(),
				updateInstr.Target.GetStructValue(),
				newValue,
				1,
			)
			updateInstr.Target.SetStructValue(newValue)
		} else {
			currLine = scanner.Text()
		}

		_, err = writer.WriteString(currLine + "\n")
		if err != nil {
			return err
		}

		currLineNumber++
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	if err := newFile.Close(); err != nil {
		return err
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if err := overwriteFile(newFile.Name(), updateInstr.File); err != nil {
		return err
	}

	return nil
}

func overwriteFile(src string, dst string) error {
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return nil
}

type VersionIter interface {
	HasNext() bool
	Next() string
}

var _ VersionIter = (*slices.Iter[string])(nil)
var _ VersionIter = (*helm.ChartVersionIter)(nil)

type Strategy interface {
	HasNewerRemoteVersion(
		currentVersion string,
		remoteVersions VersionIter,
	) (string, bool, error)
}

type SemVerStrategy struct {
	constraint string
}

func (strat *SemVerStrategy) HasNewerRemoteVersion(
	currentVersion string,
	remoteVersions VersionIter,
) (string, bool, error) {
	semverConstraint, err := semver.NewConstraint(strat.constraint)
	if err != nil {
		return "", false, err
	}

	var latestRemoteSemverVersion *semver.Version
	for remoteVersions.HasNext() {
		version := remoteVersions.Next()
		remoteVersion, err := semver.NewVersion(version)
		if err != nil || !semverConstraint.Check(remoteVersion) {
			continue
		}

		if latestRemoteSemverVersion == nil {
			latestRemoteSemverVersion = remoteVersion
			continue
		}

		if remoteVersion.GreaterThan(latestRemoteSemverVersion) {
			latestRemoteSemverVersion = remoteVersion
		}
	}

	if latestRemoteSemverVersion == nil {
		return "", false, nil
	}

	currentSemverVersion, err := semver.NewVersion(currentVersion)
	if err != nil {
		return "", false, err
	}

	if latestRemoteSemverVersion.GreaterThan(currentSemverVersion) {
		return latestRemoteSemverVersion.Original(), true, nil
	}

	return latestRemoteSemverVersion.Original(), false, nil
}

var _ Strategy = (*SemVerStrategy)(nil)

func getStrategy(strategy UpdateStrategy, constraint string) Strategy {
	switch strategy {

	case SemVer:
		return &SemVerStrategy{constraint: constraint}
	}

	return nil
}
