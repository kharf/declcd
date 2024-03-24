package prometheus

import (
	"github.com/kharf/declcd/schema@v0"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
)

release: schema.#HelmRelease & {
	dependencies: [
		ns.id,
		linkerd.ns.id,
	]
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
