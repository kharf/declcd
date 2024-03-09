package prometheus

import "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"

#namespace: v1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: "monitoring"
}
