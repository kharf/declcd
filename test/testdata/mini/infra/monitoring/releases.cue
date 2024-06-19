package monitoring

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

ns: component.#Manifest & {
	content: v1.#Namespace & {
		apiVersion: "v1"
		kind:       "Namespace"
		metadata: {
			name: "monitoring"
		}
	}
}

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "{{.Name}}"
	namespace: ns.content.metadata.name
	chart: {
		name:    "test"
		repoURL: "{{.RepoURL}}"
		version: "1.0.0"
	}
	values: {
		autoscaling: enabled: true
	}
}
