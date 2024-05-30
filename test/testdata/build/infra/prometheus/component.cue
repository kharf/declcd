package prometheus

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
			foo: '(enc;value omitted)'
		}
	}
}

release: component.#HelmRelease & {
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

releaseSecretRef: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test-secret-ref"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
		auth: secretRef: {
			name:      "secret"
			namespace: "namespace"
		}
	}
	values: {
		autoscaling: enabled: true
	}
}

releaseWorkloadIdentity: component.#HelmRelease & {
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
	}
	values: {
		autoscaling: enabled: true
	}
}
