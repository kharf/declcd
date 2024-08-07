package metadatamissing

import (
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
)

secret: {
	type: "Manifest"
	id:   "unimportant"
	dependencies: []
	content: corev1.#Secret & {
		apiVersion: "v1"
		kind:       "Secret"
		data: {
			foo: 'bar'
		}
	}
}
