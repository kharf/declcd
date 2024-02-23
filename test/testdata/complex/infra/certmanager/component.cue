package certmanager

import (
	"github.com/kharf/declcd/api/v1"
)

ns: v1.#Component & {
	content: _namespace
}

release: content: _release
