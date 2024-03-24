package emptyreleasechartnamewithschema

import (
	"github.com/kharf/declcd/schema@v0"
)

release: schema.#HelmRelease & {
	name:      "test"
	namespace: "test"
	chart: {
		name:    ""
		repoURL: "oci://"
		version: "test"
	}
	values: {
		autoscaling: enabled: true
	}
}
