package monitors

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/prometheus"
)

monitor: v1.#Component & {
	dependencies: [
		prometheus.namespace.id,
	]
	content: _declcdMonitor
}
