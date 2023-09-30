package subcomponent

import (
	"github.com/kharf/declcd/api/v1"
)

subcomponent: v1.#Component & {
	manifests: [
		_deployment,
	]
}
