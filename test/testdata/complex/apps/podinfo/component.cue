package podinfo

import "github.com/kharf/declcd/api/v1"

namespace: v1.#Component & {
	content: _namespace
}

deployment: v1.#Component & {
	content: _deployment
}
