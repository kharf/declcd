package linkerd

import "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"

_namespace: v1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name: "linkerd"
		annotations: {
			"linkerd.io/inject": "disabled"
		}
		labels: {
			"linkerd.io/is-control-plan":           "true"
			"config.linkerd.io/admission-webhooks": "disabled"
			"linkerd.io/control-plane-ns":          metadata.name
		}
	}
}
