package podinfotemp

import "github.com/kharf/declcd/api/v1"

namespace: content: _namespace

deployment: v1.#Component & {
	content: _deployment
}
