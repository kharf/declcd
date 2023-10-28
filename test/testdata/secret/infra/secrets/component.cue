package secrets

import "github.com/kharf/declcd/api/v1"

secrets: v1.#Component & {
	manifests: [
		#namespace,
		#data,
		#stringData,
		#both,
		#multiLine,
		#none,
	]
}
