package prometheus

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
)

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
		linkerd.ns.id,
	]
	name:      "{{.Name}}"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "{{.RepoURL}}"
		version: "1.0.0"
	}
	crds: {
		allowUpgrade: true
	}
	values: {
		autoscaling: enabled: true
	}
}
