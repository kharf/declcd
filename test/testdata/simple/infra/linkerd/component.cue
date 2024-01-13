package linkerd

import "github.com/kharf/declcd/api/v1"

v1.#Component & {
	linkerd: {
		manifests: [
			#namespace,
		]
		helmReleases: [
		]
	}
}
