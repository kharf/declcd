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

package version

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-logr/logr"

	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/vcs"
)

// UpdateIntegration defines the method on how to push updates to the version control system.
type UpdateIntegration int

const (
	// Direct indicates to push updates directly to the base branch and reconcile them in the same run.
	Direct UpdateIntegration = iota
	// PR indicates to push updates to a separate update branch and create a pull request. Updates are not applied immediately, only after the PR has been merged and the changes were pulled.
	PR
)

var (
	// ErrUnexpectedResponse is returned when an unexpected response is received from a repository.
	ErrUnexpectedResponse = errors.New("Unexpected response")
	// ErrChartNotFound is returned when a chart is not found in the repository.
	ErrChartNotFound = errors.New("Chart not found")
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
	imageParts := strings.Split(c.Image, ":")
	return imageParts[0]
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

func (c *ChartUpdateTarget) GetStructValue() string {
	return c.Chart.Version
}

func (c *ChartUpdateTarget) SetStructValue(newValue string) {
	c.Chart.Version = newValue
}

var _ UpdateTarget = (*ChartUpdateTarget)(nil)

// Object to be updated.
type UpdateTarget interface {
	// Name returns the name of the update target.
	// It is either a container name or a helm chart.
	Name() string
	// SetStructValue sets a new value for the struct field.
	// It is either an image field in an unstructured manifest or a version field in a helm chart.
	SetStructValue(newValue string)
	// GetStructValue retrieves the current value of the struct field.
	// It is either an image field in an unstructured manifest or a version field in a helm chart.
	GetStructValue() string
}

// UpdateInstruction represents the instruction for updating a target, such as a container image or a Helm chart.
type UpdateInstruction struct {
	// Strategy defines the method to update the target.
	Strategy UpdateStrategy
	// Constraint specifies any constraints that need to be considered during the update process.
	Constraint string
	// Auth contains authentication details required for accessing and updating the target.
	// Only relevant for manifest components. For Helm Charts, auth is taken from the component def.
	Auth *cloud.Auth

	// Integration defines the method on how to push updates to the version control system.
	Integration UpdateIntegration

	// File path where the version value is located.
	File string
	// Line number in the file where the version value resides.
	Line int

	// Target specifies what needs to be updated, which can be a container image or a Helm chart.
	// A container image follows the format 'repository:tag@digest'.
	// A Helm repository can either be of type 'oci' or 'https'.
	Target UpdateTarget
}

// Update represents the result of an update operation.
type Update struct {
	// CommitHash contains the SHA1 of the commit.
	CommitHash string

	// NewVersion contains the updated version.
	NewVersion string
}

type Updates struct {
	DirectUpdates []Update
}

// Updater accepts update instructions that tell which images to update.
// For every instruction it contacts image registries to fetch remote tags and calculates the latest tag based on the provided update strategy.
// If the latest tag is greater than the current tag, it updates the image and commits the changes.
// It pushes its changes to remote before returning.
type Updater struct {
	Log        logr.Logger
	Repository vcs.Repository
}

// Update accepts version scan results that tell which images or chart to update and returns update results.
func (updater *Updater) Update(
	ctx context.Context,
	scanResults []ScanResult,
	branch string,
) (*Updates, error) {
	var directUpdates []Update
	for _, result := range scanResults {
		if result.CurrentVersion == result.NewVersion {
			continue
		}

		targetName := result.Target.Name()

		updater.Log.Info(
			"Updating",
			"target",
			targetName,
			"newVersion",
			result.NewVersion,
			"file",
			result.File,
		)

		commitMessage := fmt.Sprintf(
			"chore(update): bump %s to %s",
			targetName,
			result.NewVersion,
		)

		switch result.Integration {
		case PR:
			src := fmt.Sprintf("declcd/update-%s", targetName)
			if err := updater.Repository.NewBranch(src); err != nil {
				return nil, err
			}

			_, err := updater.update(commitMessage, result)
			if err != nil {
				return nil, err
			}

			if err := updater.Repository.Push(src, branch); err != nil {
				return nil, err
			}

			if err := updater.Repository.CreatePullRequest(commitMessage, src, branch); err != nil {
				return nil, err
			}

			if err := updater.Repository.SwitchBranch(branch); err != nil {
				return nil, err
			}

		case Direct:
			update, err := updater.update(commitMessage, result)
			if err != nil {
				return nil, err
			}

			directUpdates = append(directUpdates, *update)
		}
	}

	if len(directUpdates) > 0 {
		if err := updater.Repository.Push(branch, branch); err != nil {
			return nil, err
		}
	}

	return &Updates{
		DirectUpdates: directUpdates,
	}, nil
}

func (updater *Updater) update(commitMessage string, result ScanResult) (*Update, error) {
	if err := updater.updateVersion(result); err != nil {
		return nil, err
	}

	hash, err := updater.Repository.Commit(result.File,
		commitMessage,
	)
	if err != nil {
		return nil, err
	}

	return &Update{
		CommitHash: hash,
		NewVersion: result.NewVersion,
	}, nil
}

func (updater *Updater) updateVersion(
	result ScanResult,
) error {
	file, err := os.Open(result.File)
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
		if currLineNumber == result.Line {
			newValue := strings.Replace(
				result.Target.GetStructValue(),
				result.CurrentVersion,
				result.NewVersion,
				1,
			)
			currLine = strings.Replace(
				scanner.Text(),
				result.Target.GetStructValue(),
				newValue,
				1,
			)
			result.Target.SetStructValue(newValue)
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

	if err := overwriteFile(newFile.Name(), result.File); err != nil {
		return err
	}

	return nil
}

func overwriteFile(src string, dst string) error {
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

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
