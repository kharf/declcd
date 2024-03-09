package prometheus

import (
	"github.com/kharf/declcd/schema@v0"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

#namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "prometheus"
	}
}

ns: schema.#Component & {
	content: #namespace
}

secret: schema.#Component & {
	dependencies: [
		ns.id,
	]
	content: corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      "secret"
			namespace: #namespace.metadata.name
		}
		data: {
			foo: '(enc;value omitted)'
		}
	}
}

release: schema.#Component & {
	dependencies: [
		ns.id,
	]
	content: schema.#HelmRelease & {
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
}
