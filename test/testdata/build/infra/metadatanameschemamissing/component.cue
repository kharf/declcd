package metadatanameschemamissing

import (
	v1 "github.com/kharf/declcd/api/v1"
	corev1 "k8s.io/api/core/v1"
)

secret: v1.#Component & {
	content: corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			namespace: "test"
		}
		data: {
			foo: '(enc;value omitted)'
		}
	}
}
