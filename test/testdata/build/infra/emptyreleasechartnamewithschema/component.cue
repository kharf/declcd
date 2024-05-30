package emptyreleasechartnamewithschema

import (
	"github.com/kharf/declcd/schema/component"
)

release: component.#HelmRelease & {
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
