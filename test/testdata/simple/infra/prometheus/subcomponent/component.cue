package subcomponent

import (
	"github.com/kharf/declcd/schema@v0"
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
)

deployment: schema.#Component & {
	dependencies: [
		prometheus.ns.id,
	]
	content: _deployment
}
