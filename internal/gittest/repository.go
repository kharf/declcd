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

package gittest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/kharf/declcd/pkg/vcs"
	"gotest.tools/v3/assert"
)

type LocalGitRepository struct {
	Worktree  *git.Worktree
	Directory string
}

func (r *LocalGitRepository) Clean() error {
	return os.RemoveAll(r.Directory)
}

func (r *LocalGitRepository) CommitFile(file string, message string) (string, error) {
	worktree := r.Worktree
	if _, err := worktree.Add(file); err != nil {
		return "", err
	}
	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func (r *LocalGitRepository) CommitNewFile(file string, message string) (string, error) {
	if err := os.WriteFile(filepath.Join(r.Directory, file), []byte{}, 0664); err != nil {
		return "", err
	}
	return r.CommitFile(file, message)
}

func SetupGitRepository() (*LocalGitRepository, error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}
	fileName := "test1"
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, err
	}
	worktree, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	localRepository := &LocalGitRepository{
		Worktree:  worktree,
		Directory: dir,
	}
	if _, err := localRepository.CommitNewFile(fileName, "first commit"); err != nil {
		return nil, err
	}
	return localRepository, nil
}

func InitGitRepository(dir string) (*LocalGitRepository, error) {
	r, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, err
	}
	worktree, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	localRepository := &LocalGitRepository{
		Worktree:  worktree,
		Directory: dir,
	}
	if _, err := localRepository.CommitFile(".", "first commit"); err != nil {
		return nil, err
	}

	return localRepository, nil
}

func OpenGitRepository(dir string) (*LocalGitRepository, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	worktree, err := r.Worktree()
	if err != nil {
		return nil, err
	}
	localRepository := &LocalGitRepository{
		Worktree:  worktree,
		Directory: dir,
	}

	return localRepository, nil
}

// enforceHostRoundTripper rewrites all requests with the given `Host`.
type enforceHostRoundTripper struct {
	Host                 string
	UpstreamRoundTripper http.RoundTripper
}

func (efrt *enforceHostRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	splitHost := strings.Split(efrt.Host, "://")
	r.URL.Scheme = splitHost[0]
	r.URL.Host = splitHost[1]

	return efrt.UpstreamRoundTripper.RoundTrip(r)
}

type request struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

func MockGitProvider(
	t *testing.T,
	provider vcs.Provider,
	expectedTitle string,
) (*httptest.Server, *http.Client) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		}
		switch provider {
		case vcs.GitHub:
			authHeader := r.Header["Authorization"]
			assert.Assert(t, len(authHeader) == 1)
			assert.Assert(t, strings.HasPrefix(authHeader[0], "Bearer"))
			assert.Assert(t, authHeader[0] != "Bearer ")
			assert.Assert(t, authHeader[0] != "Bearer")
		case vcs.GitLab:
			authHeader := r.Header["Private-Token"]
			assert.Assert(t, len(authHeader) == 1)
			assert.Assert(t, authHeader[0] != "")
		}
		var req request
		err = json.Unmarshal(bodyBytes, &req)
		assert.NilError(t, err)
		assert.Equal(t, req.Title, expectedTitle)
		assert.Assert(t, strings.HasPrefix(req.Key, "ssh-ed25519 AAAA"))
		w.Write([]byte(`{
				"key" : "ssh-rsa AAAA...",
				"id" : 12,
				"title" : "My deploy key",
				"can_push": true,
				"created_at" : "2015-08-29T12:44:31.550Z",
				"expires_at": null
				}
			}`))
	}))
	client := server.Client()
	client.Transport = &enforceHostRoundTripper{
		Host:                 server.URL,
		UpstreamRoundTripper: client.Transport,
	}
	return server, client
}
