package wrongprefixreleasecharturlwithschema

import (
	"github.com/kharf/declcd/schema/component"
)

release: component.#HelmRelease & {
	name:      "test"
	namespace: "test"
	chart: {
		name:    "test"
		repoURL: "heelloo.com"
		version: "test"
	}
	values: {
		autoscaling: enabled: true
	}
}
