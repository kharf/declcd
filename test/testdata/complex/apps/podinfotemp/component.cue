package podinfotemp

import "github.com/kharf/declcd/api/v1"

v1.#Component & {
	pit: {
		manifests: [
			_namespace,
			_deployment,
		]
	}
}
