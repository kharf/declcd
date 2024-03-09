package prometheus

import (
	"github.com/kharf/declcd/schema@v0"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
)

ns: schema.#Component & {
	content: #namespace
}

secret: schema.#Component & {
	dependencies: [
		ns.id,
	]
	content: _secret
}

release: schema.#Component & {
	dependencies: [
		ns.id,
		linkerd.ns.id,
	]
	content: _release
}
