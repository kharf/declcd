package metadatamissing

import (
	corev1 "k8s.io/api/core/v1"
)

secret: {
	type: "Manifest"
	dependencies: [
	]
	content: corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		data: {
			foo: '(enc;value omitted)'
		}
	}
}
