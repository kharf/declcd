package pyroscope

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/prometheus"
)

v1.#Component & {
	pyroscope: {
		dependencies: [
			prometheus.prometheus.id,
		]
		manifests: [
		]
		helmReleases: [
			_release,
		]
	}
}
