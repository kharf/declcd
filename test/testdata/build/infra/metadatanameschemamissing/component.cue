package metadatanameschemamissing

import (
	"github.com/kharf/declcd/schema/component"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

secret: component.#Manifest & {
	content: corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			namespace: "test"
		}
		data: {
			foo: 'bar'
		}
	}
}
