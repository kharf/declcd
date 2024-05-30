package emptyreleasenamewithschema

import (
	"github.com/kharf/declcd/schema/component"
)

release: component.#HelmRelease & {
	name:      ""
	namespace: "test"
	chart: {
		name:    "test"
		repoURL: "http://test"
		version: "test"
	}
	values: {
		autoscaling: enabled: true
	}
}
