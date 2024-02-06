package certmanager

import (
	"github.com/kharf/declcd/api/v1"
)

v1.#Component & {
	certManager: {
		manifests: [
			_namespace,
		]
		helmReleases: [
			_release,
		]
	}
}
