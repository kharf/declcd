package prometheus

import (
	"github.com/kharf/declcd/schema@v0"
)

ns: schema.#Manifest & {
	content: #namespace
}

secret: schema.#Manifest & {
	dependencies: [
		ns.id,
	]
	content: _secret
}
