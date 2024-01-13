package prometheus

import "k8s.io/api/core/v1"

_secret: v1.#Secret & {
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
