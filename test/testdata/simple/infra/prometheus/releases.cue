package prometheus

import (
	"github.com/kharf/declcd/schema@v0"
)

_release: schema.#HelmRelease & {
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
