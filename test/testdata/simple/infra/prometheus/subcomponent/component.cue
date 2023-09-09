package subcomponent

import (
	"github.com/kharf/declcd/api/v1"
)

subcomponent: v1.#Component & {
	intervalSeconds: 1
	manifests: [
		_deployment,
	]
}
