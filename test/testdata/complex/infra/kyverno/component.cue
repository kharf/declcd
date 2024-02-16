package kyverno

import (
	corev1 "k8s.io/api/core/v1"
	"github.com/kharf/declcd/api/v1"
)

_release: v1.#HelmRelease & {
	name:      "kyverno"
	namespace: _namespace.metadata.name
	chart: {
		name:    "kyverno"
		repoURL: "oci://ghcr.io/kyverno/charts"
		version: "3.1.4"
	}
}

_namespace: corev1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "kyverno"
	}
}

ns: content:      _namespace
release: content: _release
