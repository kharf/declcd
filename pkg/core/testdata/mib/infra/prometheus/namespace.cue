package prometheus

import "k8s.io/api/core/v1"

namespace: mynamespace: v1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: "mynamespace"
}
