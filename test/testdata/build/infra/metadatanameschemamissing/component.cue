package metadatanameschemamissing

import (
	"github.com/kharf/declcd/schema@v0"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

secret: schema.#Component & {
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
