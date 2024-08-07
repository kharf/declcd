package container

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/kubernetes/pkg/util/parsers"
)

type Update struct {
	CommitHash string
	Image      string
	NewVersion string
	Line       int
}

type UpdateResult struct {
	Updates []Update
}

type Updater struct {
	GitRepository *git.Repository
}

func (updater *Updater) Update(components []component.Instance) (*UpdateResult, error) {
	var updates []Update
	for _, componentInstances := range components {
		switch actualInstance := componentInstances.(type) {
		case *kube.Manifest:
			content := actualInstance.Content
			if content.Metadata == nil {
				continue
			}

			manifestUpdates, err := updater.update(*content.Metadata)
			if err != nil {
				return nil, err
			}
			updates = append(updates, manifestUpdates...)

		case *helm.ReleaseComponent:
			for _, unstr := range actualInstance.Content.Patches.Unstructureds {
				if unstr.Metadata == nil {
					continue
				}

				hrUpdates, err := updater.update(*unstr.Metadata)
				if err != nil {
					return nil, err
				}
				updates = append(updates, hrUpdates...)
			}
		}
	}

	return &UpdateResult{
		Updates: updates,
	}, nil
}

func (updater *Updater) update(metadata kube.ManifestMetadata) ([]Update, error) {
	var updates []Update
	if metadata.Field != nil && metadata.Field.UpdateAttr != nil {
		repoName, tag, _, err := parsers.ParseImageName(metadata.Field.UpdateAttr.Image)
		if err != nil {
			return nil, err
		}

		repository, err := name.NewRepository(repoName)
		if err != nil {
			return nil, err
		}

		remoteTags, err := remote.List(repository, remote.WithPageSize(5))
		if err != nil {
			return nil, err
		}

		switch metadata.Field.UpdateAttr.Strategy {
		case kube.Semver:
			constaint, err := semver.NewConstraint(metadata.Field.UpdateAttr.Constraint)
			if err != nil {
				return nil, err
			}

			currentVersion, err := semver.NewVersion(tag)
			if err != nil {
				return nil, err
			}

			remoteVersions := make([]*semver.Version, 0, len(remoteTags))
			for _, remoteTag := range remoteTags {
				remoteVersion, err := semver.NewVersion(remoteTag)
				if err != nil {
					continue
				}

				if !constaint.Check(remoteVersion) {
					continue
				}
				remoteVersions = append(remoteVersions, remoteVersion)
			}

			if len(remoteVersions) == 0 {
				return nil, nil
			}

			sort.Sort(semver.Collection(remoteVersions))

			maxRemoteVersion := remoteVersions[len(remoteVersions)-1]

			if maxRemoteVersion.GreaterThan(currentVersion) {
				// update
				file, err := os.Open(metadata.Field.UpdateAttr.File)
				if err != nil {
					return nil, err
				}
				defer file.Close()

				newFile, err := os.CreateTemp("", "container-update-*")
				if err != nil {
					return nil, err
				}
				defer func() {
					_ = os.Remove(newFile.Name())
				}()

				scanner := bufio.NewScanner(file)
				writer := bufio.NewWriter(newFile)

				currLineNumber := 1
				for scanner.Scan() {
					var currLine string
					if currLineNumber == metadata.Field.UpdateAttr.Line {
						newImage := fmt.Sprintf("%s:%s", repoName, maxRemoteVersion.Original())
						currLine = strings.ReplaceAll(
							scanner.Text(),
							metadata.Field.UpdateAttr.Image,
							newImage,
						)
					} else {
						currLine = scanner.Text()
					}

					_, err = writer.WriteString(currLine + "\n")
					if err != nil {
						return nil, err
					}

					currLineNumber++
				}
				if err := writer.Flush(); err != nil {
					return nil, err
				}

				if err := newFile.Close(); err != nil {
					return nil, err
				}

				if err := scanner.Err(); err != nil {
					return nil, err
				}

				originalFile, err := os.Create(metadata.Field.UpdateAttr.File)
				if err != nil {
					return nil, err
				}

				newFile, err = os.Open(newFile.Name())
				if err != nil {
					return nil, err
				}
				defer newFile.Close()
				if _, err := io.Copy(originalFile, newFile); err != nil {
					return nil, err
				}

				worktree, err := updater.GitRepository.Worktree()
				if err != nil {
					return nil, err
				}

				relPath, err := filepath.Rel(
					worktree.Filesystem.Root(),
					metadata.Field.UpdateAttr.File,
				)
				if err != nil {
					return nil, err
				}

				_, err = worktree.Add(relPath)
				if err != nil {
					return nil, err
				}

				newVersion := maxRemoteVersion.Original()

				hash, err := worktree.Commit(
					fmt.Sprintf(
						"chore(image): bump %s to %s",
						repoName,
						newVersion,
					),
					&git.CommitOptions{
						Author: &object.Signature{
							Name: "declcd-bot",
						},
					},
				)
				if err != nil {
					return nil, err
				}

				if err := updater.GitRepository.Push(&git.PushOptions{}); err != nil {
					return nil, err
				}

				update := Update{
					CommitHash: hash.String(),
					Image:      repoName,
					NewVersion: newVersion,
					Line:       metadata.Field.UpdateAttr.Line,
				}

				updates = append(updates, update)
			}
		}

	}

	for _, metadata := range metadata.Node {
		nodeUpdates, err := updater.update(metadata)
		if err != nil {
			return nil, err
		}
		updates = append(updates, nodeUpdates...)
	}

	for _, metadata := range metadata.List {
		listUpdates, err := updater.update(metadata)
		if err != nil {
			return nil, err
		}
		updates = append(updates, listUpdates...)
	}

	return updates, nil
}
