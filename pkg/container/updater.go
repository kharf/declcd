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

package container

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/vcs"
	"k8s.io/kubernetes/pkg/util/parsers"
)

// Update represents the result of an update operation.
type Update struct {
	// CommitHash contains the SHA1 of the commit.
	CommitHash string

	// Image contains the updated image, including the repository.
	Image string

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
func (updater *Updater) Update(updateInstructions []kube.UpdateInstruction) ([]Update, error) {
	var updates []Update
	for _, updateInstr := range updateInstructions {
		repoName, currentTag, _, err := parsers.ParseImageName(updateInstr.Image)
		if err != nil {
			return nil, err
		}

		repository, err := name.NewRepository(repoName)
		if err != nil {
			return nil, err
		}

		remoteTags, err := remote.List(repository)
		if err != nil {
			return nil, err
		}

		switch updateInstr.Strategy {
		case kube.Semver:
			currentVersion, err := semver.NewVersion(currentTag)
			if err != nil {
				return nil, err
			}

			latestRemoteVersion, err := getLatestRemoteVersion(updateInstr.Constraint, remoteTags)
			if err != nil {
				return nil, err
			}

			if latestRemoteVersion == nil {
				continue
			}

			if latestRemoteVersion.GreaterThan(currentVersion) {
				newVersion := latestRemoteVersion.Original()
				updater.Log.Info(
					"Updating container image",
					"image",
					updateInstr.Image,
					"newVersion",
					newVersion,
					"file",
					updateInstr.File,
				)

				if err := updater.updateImage(updateInstr, repoName, newVersion); err != nil {
					return nil, err
				}

				hash, err := updater.Repository.Commit(updateInstr.File,
					fmt.Sprintf(
						"chore(image): bump %s to %s",
						repoName,
						newVersion,
					),
				)
				if err != nil {
					return nil, err
				}

				updates = append(updates, Update{
					CommitHash: hash,
					Image:      repoName,
					NewVersion: newVersion,
				})
			}
		}
	}

	if len(updates) > 0 {
		if err := updater.Repository.Push(); err != nil {
			return nil, err
		}
	}

	return updates, nil
}

func getLatestRemoteVersion(
	constraint string,
	remoteTags []string,
) (*semver.Version, error) {
	if len(remoteTags) == 0 {
		return nil, nil
	}

	semverConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, err
	}

	var latestRemoteVersion *semver.Version
	for _, remoteTag := range remoteTags {
		remoteVersion, err := semver.NewVersion(remoteTag)
		if err != nil || !semverConstraint.Check(remoteVersion) {
			continue
		}

		if latestRemoteVersion == nil {
			latestRemoteVersion = remoteVersion
			continue
		}

		if remoteVersion.GreaterThan(latestRemoteVersion) {
			latestRemoteVersion = remoteVersion
		}
	}

	return latestRemoteVersion, nil
}

func (updater *Updater) updateImage(
	updateInstr kube.UpdateInstruction,
	repoName string,
	newVersion string,
) error {
	file, err := os.Open(updateInstr.File)
	if err != nil {
		return err
	}
	defer file.Close()

	newFile, err := os.CreateTemp("", "container-update-*")
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
			newImage := fmt.Sprintf("%s:%s", repoName, newVersion)
			currLine = strings.ReplaceAll(
				scanner.Text(),
				updateInstr.Image,
				newImage,
			)
			updateInstr.UnstructuredNode[updateInstr.UnstructuredKey] = newImage
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
