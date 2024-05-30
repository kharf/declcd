package emptyreleasechartversionwithschema

import (
	"github.com/kharf/declcd/schema/component"
)

release: component.#HelmRelease & {
	name:      "test"
	namespace: "test"
	chart: {
		name:    "test"
		repoURL: "https://test"
		version: ""
	}
	values: {
		autoscaling: enabled: true
	}
}
