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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/kharf/declcd/internal/slices"
	"github.com/kharf/declcd/pkg/cloud"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"
	"k8s.io/kubernetes/pkg/util/parsers"
	"sigs.k8s.io/yaml"
)

// Scanner is the system for performing version scanning operations.
type Scanner struct {
	Log    logr.Logger
	Client *kube.DynamicClient

	// Kubernetes namespace where the registry credential secret is stored.
	Namespace string

	// Endpoint to the microsoft azure login server.
	// Default is usually: https://login.microsoftonline.com/.
	AzureLoginURL string

	// Endpoint to the google metadata server, which provides access tokens.
	// Default is: http://metadata.google.internal.
	GCPMetadataServerURL string
}

// ScanResult represents the result of a version scanning operation.
// It holds details about the current and new versions, as well as the file and line at which these versions were found.
type ScanResult struct {
	// The current version that is being scanned for updates.
	CurrentVersion string
	// The new version that has been found.
	NewVersion string

	// File where the versions were found.
	File string
	// Line number within the file where the versions were found.
	Line   int
	Target UpdateTarget
}

func (scanner *Scanner) Scan(
	ctx context.Context,
	updateInstructions []UpdateInstruction,
) ([]ScanResult, error) {
	var results []ScanResult
	for _, updateInstr := range updateInstructions {
		strategy := getStrategy(updateInstr.Strategy, updateInstr.Constraint)

		versionIter, currentVersion, err := scanner.listVersions(ctx, updateInstr)
		if err != nil {
			return nil, err
		}

		newVersion, hasNewVersion, err := strategy.HasNewerRemoteVersion(
			currentVersion,
			versionIter,
		)
		if err != nil {
			return nil, err
		}
		if !hasNewVersion {
			continue
		}

		results = append(results, ScanResult{
			CurrentVersion: currentVersion,
			NewVersion:     newVersion,
			File:           updateInstr.File,
			Line:           updateInstr.Line,
			Target:         updateInstr.Target,
		})
	}

	return results, nil
}

func (scanner *Scanner) listVersions(
	ctx context.Context,
	updateInstr UpdateInstruction,
) (VersionIter, string, error) {
	var versionIter VersionIter
	var currentVersion string
	var err error

	switch target := updateInstr.Target.(type) {

	case *ContainerUpdateTarget:
		var repo string
		repo, currentVersion, _, err = parsers.ParseImageName(target.Image)
		if err != nil {
			return nil, "", err
		}
		idx := strings.LastIndex(repo, "/")
		host := repo[:idx]

		versionIter, err = scanner.listContainerVersions(
			ctx,
			host,
			repo,
			updateInstr.Auth,
		)

	case *ChartUpdateTarget:
		currentVersion = target.Chart.Version
		versionIter, err = scanner.listHelmChartVersions(ctx, target)
	}

	return versionIter, currentVersion, err
}

func (scanner *Scanner) listHelmChartVersions(
	ctx context.Context,
	target *ChartUpdateTarget,
) (VersionIter, error) {
	if registry.IsOCI(target.Chart.RepoURL) {
		host, _ := strings.CutPrefix(target.Chart.RepoURL, "oci://")
		repo := fmt.Sprintf(
			"%s/%s",
			host,
			target.Chart.Name,
		)

		return scanner.listContainerVersions(
			ctx,
			host,
			repo,
			target.Chart.Auth,
		)
	}

	return scanner.listHTTPHelmChartVersions(
		ctx,
		target.Chart.RepoURL,
		target.Chart.Name,
		target.Chart.Auth,
	)
}

func (scanner *Scanner) listContainerVersions(
	ctx context.Context,
	host string,
	repoName string,
	auth *cloud.Auth,
) (VersionIter, error) {
	repository, err := name.NewRepository(repoName)
	if err != nil {
		return nil, err
	}

	authOptions := []remote.Option{}
	if auth != nil {
		creds, err := cloud.ReadCredentials(
			ctx,
			host,
			*auth,
			scanner.Client,
			cloud.WithNamespace(scanner.Namespace),
			cloud.WithCustomAzureLoginURL(scanner.AzureLoginURL),
			cloud.WithCustomGCPMetadataServerURL(scanner.GCPMetadataServerURL),
		)
		if err != nil {
			return nil, err
		}
		authOptions = append(authOptions, remote.WithAuth(&authn.Basic{
			Username: creds.Username,
			Password: creds.Password,
		}))
	}

	remoteVersions, err := remote.List(repository, authOptions...)
	if err != nil {
		return nil, err
	}
	return &slices.Iter[string]{Slice: remoteVersions}, nil
}

func (scanner *Scanner) listHTTPHelmChartVersions(
	ctx context.Context,
	repoURL string,
	name string,
	auth *cloud.Auth,
) (VersionIter, error) {
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/index.yaml", repoURL), nil)
	if err != nil {
		return nil, err
	}

	if auth != nil {
		creds, err := cloud.ReadCredentials(
			ctx,
			repoURL,
			*auth,
			scanner.Client,
			cloud.WithNamespace(scanner.Namespace),
			cloud.WithCustomAzureLoginURL(scanner.AzureLoginURL),
			cloud.WithCustomGCPMetadataServerURL(scanner.GCPMetadataServerURL),
		)
		if err != nil {
			return nil, err
		}

		request.SetBasicAuth(creds.Username, creds.Password)
	}

	resp, err := http.DefaultClient.Do(request)
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

	var indexFile repo.IndexFile
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(bytes, &indexFile); err != nil {
		return nil, err
	}

	chartVersions, found := indexFile.Entries[name]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrChartNotFound, name)
	}

	return &helm.ChartVersionIter{Versions: chartVersions}, nil
}

type VersionIter interface {
	HasNext() bool
	Next() string
}

var _ VersionIter = (*slices.Iter[string])(nil)
var _ VersionIter = (*helm.ChartVersionIter)(nil)
