package prometheus

import "github.com/kharf/declcd/api/v1"

prometheus: v1.#Component & {
	intervalSeconds: 1
	manifests: [
		#namespace,
	]
	helmReleases: [
		_release,
	]
}
