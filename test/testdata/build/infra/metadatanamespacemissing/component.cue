package prometheus

import (
	v1 "github.com/kharf/declcd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

#namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "prometheus"
	}
}

ns: v1.#Component & {
	content: #namespace
}

secret: v1.#Component & {
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

release: v1.#Component & {
	dependencies: [
		ns.id,
	]
	content: v1.#HelmRelease & {
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
