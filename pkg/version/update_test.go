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

package version_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-logr/logr"
	"github.com/kharf/declcd/internal/gittest"
	inttxtar "github.com/kharf/declcd/internal/txtar"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/vcs"
	"github.com/kharf/declcd/pkg/version"
	"golang.org/x/tools/txtar"
	"gotest.tools/v3/assert"
)

type updateTestCase struct {
	name                    string
	haveFiles               string
	haveScanResults         []version.ScanResult
	haveBranches            []string
	haveBranchesWithChanges map[string]string
	havePullRequests        []vcs.PullRequestRequest
	wantUpdates             []version.Update
	wantPullRequests        []vcs.PullRequestRequest
	wantFiles               string
	wantErr                 string
}

var (
	updates = updateTestCase{
		name: "Updates",
		haveFiles: `
-- apps/myapp.cue --
image: "myimage:1.15.0"
image2: "myimage:1.16.5"
version: "1.15.0"
version2: "1.15.0"
image3: "myimage3:1.16.5"
`,
		wantFiles: `
-- apps/myapp.cue --
image: "myimage:1.16.5"
image2: "myimage:1.16.5"
version: "1.17.0"
version2: "1.15.0"
image3: "myimage3:1.16.5"
`,
		haveScanResults: []version.ScanResult{
			{
				CurrentVersion: "1.15.0",
				NewVersion:     "1.16.5",
				File:           "apps/myapp.cue",
				Line:           1,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.15.0",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.15.0",
					},
					UnstructuredKey: "image",
				},
			},
			{
				CurrentVersion: "1.16.5",
				NewVersion:     "1.16.5",
				File:           "apps/myapp.cue",
				Line:           2,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.16.5",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.16.5",
					},
					UnstructuredKey: "image",
				},
			},
			{
				CurrentVersion: "1.15.0",
				NewVersion:     "1.17.0",
				File:           "apps/myapp.cue",
				Line:           3,
				Target: &version.ChartUpdateTarget{
					Chart: &helm.Chart{
						Name:    "mychart",
						RepoURL: "oci://",
						Version: "1.15.0",
						Auth:    nil,
					},
				},
			},
			{
				CurrentVersion: "1.15.0",
				NewVersion:     "1.17.0",
				Integration:    version.PR,
				File:           "apps/myapp.cue",
				Line:           4,
				Target: &version.ChartUpdateTarget{
					Chart: &helm.Chart{
						Name:    "mychart2",
						RepoURL: "oci://",
						Version: "1.15.0",
						Auth:    nil,
					},
				},
			},
			{
				CurrentVersion: "1.16.5",
				NewVersion:     "1.17.0",
				Integration:    version.PR,
				File:           "apps/myapp.cue",
				Line:           5,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage3:1.16.5",
					UnstructuredNode: map[string]any{
						"image": "myimage3:1.16.5",
					},
					UnstructuredKey: "image",
				},
			},
		},
		wantUpdates: []version.Update{
			{
				CommitHash: "",
				NewVersion: "1.16.5",
			},
			{
				CommitHash: "",
				NewVersion: "1.17.0",
			},
		},
		wantPullRequests: []vcs.PullRequestRequest{
			{
				RepoID:     vcs.DefaultRepoID,
				Title:      "chore(update): bump mychart2 to 1.17.0",
				Branch:     "declcd/update-mychart2",
				BaseBranch: "main",
			},
			{
				RepoID:     vcs.DefaultRepoID,
				Title:      "chore(update): bump myimage3 to 1.17.0",
				Branch:     "declcd/update-myimage3",
				BaseBranch: "main",
			},
		},
	}

	existingBranch = updateTestCase{
		name: "Existing-Branch",
		haveFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
`,
		wantFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
`,
		haveScanResults: []version.ScanResult{
			{
				CurrentVersion: "1.14.0",
				NewVersion:     "1.15.0",
				Integration:    version.PR,
				File:           "apps/myapp.cue",
				Line:           1,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.14.0",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.14.0",
					},
					UnstructuredKey: "image",
				},
			},
		},
		haveBranches: []string{"declcd/update-myimage"},
		wantPullRequests: []vcs.PullRequestRequest{
			{
				RepoID:     vcs.DefaultRepoID,
				Title:      "chore(update): bump myimage to 1.15.0",
				Branch:     "declcd/update-myimage",
				BaseBranch: "main",
			},
		},
	}

	existingBranchWithChanges = updateTestCase{
		name: "Existing-Branch-With-Changes",
		haveFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
`,
		wantFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
`,
		haveBranchesWithChanges: map[string]string{
			"declcd/update-myimage": `
-- apps/myapp.cue --
image: "myimage:1.15.0"
`,
		},
		haveScanResults: []version.ScanResult{
			{
				CurrentVersion: "1.14.0",
				NewVersion:     "1.15.0",
				Integration:    version.PR,
				File:           "apps/myapp.cue",
				Line:           1,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.14.0",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.14.0",
					},
					UnstructuredKey: "image",
				},
			},
		},
		wantPullRequests: []vcs.PullRequestRequest{
			{
				RepoID:     vcs.DefaultRepoID,
				Title:      "chore(update): bump myimage to 1.15.0",
				Branch:     "declcd/update-myimage",
				BaseBranch: "main",
			},
		},
	}

	existingPullRequest = updateTestCase{
		name: "Existing-PullRequest",
		haveFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
image: "myimage:1.14.0"
`,
		wantFiles: `
-- apps/myapp.cue --
image: "myimage:1.14.0"
image: "myimage:1.15.0"
`,
		havePullRequests: []vcs.PullRequestRequest{
			{
				Branch:     "declcd/update-myimage",
				BaseBranch: "main",
			},
		},
		wantPullRequests: []vcs.PullRequestRequest{
			{
				RepoID:     vcs.DefaultRepoID,
				Title:      "chore(update): bump myimage to 1.15.0",
				Branch:     "declcd/update-myimage",
				BaseBranch: "main",
			},
		},
		haveScanResults: []version.ScanResult{
			{
				CurrentVersion: "1.14.0",
				NewVersion:     "1.15.0",
				Integration:    version.PR,
				File:           "apps/myapp.cue",
				Line:           1,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.14.0",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.14.0",
					},
					UnstructuredKey: "image",
				},
			},
			{
				CurrentVersion: "1.14.0",
				NewVersion:     "1.15.0",
				Integration:    version.Direct,
				File:           "apps/myapp.cue",
				Line:           2,
				Target: &version.ContainerUpdateTarget{
					Image: "myimage:1.14.0",
					UnstructuredNode: map[string]any{
						"image": "myimage:1.14.0",
					},
					UnstructuredKey: "image",
				},
			},
		},
		wantUpdates: []version.Update{
			{
				CommitHash: "",
				NewVersion: "1.15.0",
			},
		},
	}
)

func TestUpdater_Update(t *testing.T) {
	ctx := context.Background()

	testCases := []updateTestCase{
		updates,
		existingBranch,
		existingBranchWithChanges,
		existingPullRequest,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runUpdateTestCase(t, ctx, tc)
		})
	}
}

func runUpdateTestCase(t *testing.T, ctx context.Context, tc updateTestCase) {
	projectDir := t.TempDir()

	haveArch, err := inttxtar.Create(projectDir, bytes.NewReader([]byte(tc.haveFiles)))
	assert.NilError(t, err)

	server, client := gittest.MockGitProvider(
		t,
		vcs.DefaultRepoID,
		fmt.Sprintf("declcd-%s", `dev`),
		tc.wantPullRequests,
		tc.havePullRequests,
	)
	defer server.Close()

	remoteDir := t.TempDir()
	gitRepo, err := gittest.InitGitRepository(t, remoteDir, projectDir, "main")
	assert.NilError(t, err)

	vcsRepository, err := vcs.Open(projectDir, vcs.WithAuth(vcs.Auth{
		Method: nil,
		Token:  "",
	}), vcs.WithProvider(vcs.GitHub), vcs.WithHTTPClient(client))
	assert.NilError(t, err)

	for _, branch := range tc.haveBranches {
		err = vcsRepository.SwitchBranch(branch, true)
		assert.NilError(t, err)
	}

	for branch, files := range tc.haveBranchesWithChanges {
		err = vcsRepository.SwitchBranch(branch, true)
		assert.NilError(t, err)
		_, err := inttxtar.Create(projectDir, bytes.NewReader([]byte(files)))
		assert.NilError(t, err)
		_, err = gitRepo.CommitFile(".", "update")
		assert.NilError(t, err)
	}

	err = vcsRepository.SwitchBranch("main", false)
	assert.NilError(t, err)

	updater := &version.Updater{
		Log:        logr.Discard(),
		Repository: vcsRepository,
	}

	patchedResults := make([]version.ScanResult, 0, len(tc.haveScanResults))
	for _, result := range tc.haveScanResults {
		patchedResults = append(patchedResults, patchFile(result, projectDir))
	}

	updates, err := updater.Update(ctx, patchedResults, "main")
	if tc.wantErr != "" {
		assert.ErrorContains(t, err, tc.wantErr)
		return
	}
	assert.NilError(t, err)

	assert.Equal(t, len(updates.DirectUpdates), len(tc.wantUpdates))
	assert.Assert(t, slices.CompareFunc(
		updates.DirectUpdates,
		tc.wantUpdates,
		func(current version.Update, expected version.Update) int {
			if current.NewVersion == expected.NewVersion {
				return 0
			}

			return -1
		},
	) == 0)

	wantArch, err := inttxtar.Create(t.TempDir(), bytes.NewReader([]byte(tc.wantFiles)))
	assert.NilError(t, err)

	assert.Equal(
		t,
		len(haveArch.Files),
		len(wantArch.Files),
		"wrong testcase: file count of haveFiles and wantFiles should not differ",
	)

	for _, result := range patchedResults {
		switch target := result.Target.(type) {
		case *version.ContainerUpdateTarget:
			split := strings.Split(target.Image, ":")
			assert.Equal(t, target.GetStructValue(), fmt.Sprintf("%s:%s", split[0], result.NewVersion))
		case *version.ChartUpdateTarget:
			assert.Equal(t, target.GetStructValue(), result.NewVersion)
		}

		haveFile, err := os.Open(result.File)
		assert.NilError(t, err)
		haveData, err := io.ReadAll(haveFile)
		assert.NilError(t, err)
		haveFileName, err := filepath.Rel(projectDir, haveFile.Name())
		assert.NilError(t, err)

		assert.Assert(t, slices.ContainsFunc(wantArch.Files, func(wantFile txtar.File) bool {
			return wantFile.Name == haveFileName
		}))

		for _, wantFile := range wantArch.Files {
			if wantFile.Name == haveFileName {
				assert.Equal(t, string(haveData), string(wantFile.Data))
			}
		}
	}

	gitRepository, err := git.PlainClone(t.TempDir(), false,
		&git.CloneOptions{
			URL:           remoteDir,
			Progress:      nil,
			ReferenceName: plumbing.ReferenceName("main"),
		},
	)
	assert.NilError(t, err)

	remote, err := gitRepository.Remote(git.DefaultRemoteName)
	assert.NilError(t, err)
	remoteRefs, err := remote.List(&git.ListOptions{})
	assert.NilError(t, err)

	// check if prs are pushed to remote
	for _, pr := range tc.wantPullRequests {
		assert.Assert(
			t,
			slices.ContainsFunc(remoteRefs, func(ref *plumbing.Reference) bool {
				return pr.Branch == ref.Name().Short() && ref.Name().IsBranch()
			}),
		)
	}
}

func patchFile(result version.ScanResult, dir string) version.ScanResult {
	result.File = filepath.Join(dir, result.File)
	return result
}
