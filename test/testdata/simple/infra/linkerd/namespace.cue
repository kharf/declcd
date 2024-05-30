package linkerd

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

#namespace: v1.#Namespace & {
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: name: "linkerd"
}

ns: component.#Manifest & {
	content: #namespace
}
