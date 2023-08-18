package prometheus

import (
	"github.com/kharf/declcd/api/v1"
)

_release: v1.#HelmRelease & {
	name:      "{{.Name}}"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "{{.RepoUrl}}"
		version: "{{.Version}}"
	}
	values: {
		autoscaling: enabled: true
	}
}
