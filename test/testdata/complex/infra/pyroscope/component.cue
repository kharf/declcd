package pyroscope

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/prometheus"
)

release: v1.#Component & {
	dependencies: [
		prometheus.namespace.id,
	]
	content: _release
}
