package secrets

import "k8s.io/api/core/v1"

#b: v1.#Secret & {
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	}
	stringData: {
		foo: _bSecret
	}
}
