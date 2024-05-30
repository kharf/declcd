package subcomponent

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
)

deployment: component.#Manifest & {
	dependencies: [
		prometheus.ns.id,
	]
	content: _deployment
}
