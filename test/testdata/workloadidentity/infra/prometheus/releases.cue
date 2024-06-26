package prometheus

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/schema/workloadidentity"
)

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "{{.Name}}"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "{{.RepoURL}}"
		version: "1.0.0"
		auth:    workloadidentity.#Azure
	}
	values: {
		autoscaling: enabled: true
	}
}
