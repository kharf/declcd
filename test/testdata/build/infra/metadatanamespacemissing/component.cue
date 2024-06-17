package prometheus

import (
	"github.com/kharf/declcd/schema/component"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

#namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "prometheus"
	}
}

ns: component.#Manifest & {
	content: #namespace
}

secret: component.#Manifest & {
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
			foo: 'bar'
		}
	}
}

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
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
