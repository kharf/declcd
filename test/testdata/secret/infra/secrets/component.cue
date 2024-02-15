package secrets

import "github.com/kharf/declcd/api/v1"

v1.#Component & {
	secrets: {
		manifests: [
			#namespace,
			#data,
			#stringData,
			#both,
			#multiLine,
			#none,
			#a,
			#b,
			#c,
		]
	}
}
