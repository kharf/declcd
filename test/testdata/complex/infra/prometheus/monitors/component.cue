package monitors

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/prometheus"
)

v1.#Component & {
	monitors: {
		dependencies: [
			prometheus.prometheus.id,
		]
		manifests: [
			_declcdMonitor,
		]
		helmReleases: [
		]
	}
}
