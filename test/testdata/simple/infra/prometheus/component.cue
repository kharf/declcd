package prometheus

import (
	"github.com/kharf/declcd/schema/component"
)

ns: component.#Manifest & {
	content: #namespace
}

secret: component.#Manifest & {
	dependencies: [
		ns.id,
	]
	content: _secret
}
