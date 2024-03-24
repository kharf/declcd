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

ns: schema.#Manifest & {
	content: #namespace
}

secret: schema.#Manifest & {
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

release: schema.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
	}
	values: {
		autoscaling: enabled: true
	}
}
