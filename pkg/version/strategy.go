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
	"github.com/Masterminds/semver/v3"
)

// UpdateStrategy defines the container image or helm chart update strategy to calculate the latest version.
type UpdateStrategy int

const (
	// Semantic Versioning as defined in https://semver.org/.
	SemVer UpdateStrategy = iota
)

type Strategy interface {
	HasNewerRemoteVersion(
		currentVersion string,
		remoteVersions VersionIter[string],
	) (string, bool, int, error)
}

func getStrategy(strategy UpdateStrategy, constraint string) Strategy {
	switch strategy {

	case SemVer:
		return &SemVerStrategy{constraint: constraint}
	}

	return nil
}

// Semantic Versioning as defined in https://semver.org/.
type SemVerStrategy struct {
	// https://github.com/Masterminds/semver?tab=readme-ov-file#checking-version-constraints
	constraint string
}

func (strat *SemVerStrategy) HasNewerRemoteVersion(
	currentVersion string,
	remoteVersions VersionIter[string],
) (string, bool, int, error) {
	semverConstraint, err := semver.NewConstraint(strat.constraint)
	if err != nil {
		return "", false, 0, err
	}

	var latestRemoteSemverVersion *semver.Version
	var latestRemoteVersionIdx int

	remoteVersions.ForEach(func(version string, idx int) {
		remoteVersion, err := semver.NewVersion(version)
		if err != nil || !semverConstraint.Check(remoteVersion) {
			return
		}

		if latestRemoteSemverVersion == nil {
			latestRemoteSemverVersion = remoteVersion
			latestRemoteVersionIdx = idx
			return
		}

		if remoteVersion.GreaterThan(latestRemoteSemverVersion) {
			latestRemoteSemverVersion = remoteVersion
			latestRemoteVersionIdx = idx
		}
	})

	if latestRemoteSemverVersion == nil {
		return "", false, 0, err
	}

	currentSemverVersion, err := semver.NewVersion(currentVersion)
	if err != nil {
		return "", false, 0, err
	}

	if latestRemoteSemverVersion.GreaterThan(currentSemverVersion) {
		return latestRemoteSemverVersion.Original(), true, latestRemoteVersionIdx, nil
	}

	return latestRemoteSemverVersion.Original(), false, latestRemoteVersionIdx, nil
}

var _ Strategy = (*SemVerStrategy)(nil)
