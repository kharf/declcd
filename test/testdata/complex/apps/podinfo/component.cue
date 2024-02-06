package podinfo

import "github.com/kharf/declcd/api/v1"

v1.#Component & {
	podinfo: {
		manifests: [
			_namespace,
			_deployment,
		]
	}
}
