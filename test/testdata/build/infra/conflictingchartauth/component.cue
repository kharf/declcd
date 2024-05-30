package conflictingchartauth

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/schema/workloadidentity"
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

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test-workload-identity"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
		auth:    workloadidentity.#GCP
		auth: secretRef: {
			name:      "no"
			namespace: "no"
		}
	}
	values: {
		autoscaling: enabled: true
	}
}
