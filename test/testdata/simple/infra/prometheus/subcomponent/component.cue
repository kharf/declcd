package subcomponent

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
)

deployment: v1.#Component & {
	dependencies: [
		prometheus.ns.id,
	]
	content: _deployment
}
