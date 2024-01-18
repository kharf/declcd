package subcomponent

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
)

v1.#Component & {
	subcomponent: {
		dependencies: [
			prometheus.prometheus.id,
		]
		manifests: [
			_deployment,
		]
	}
}
