package prometheus

import (
	v1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
)

ns: v1.#Component & {
	content: #namespace
}

secret: v1.#Component & {
	dependencies: [
		ns.id,
	]
	content: _secret
}

release: v1.#Component & {
	dependencies: [
		ns.id,
		linkerd.ns.id,
	]
	content: _release
}
