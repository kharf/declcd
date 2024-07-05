package allowcrdsupgrade

import (
	"github.com/kharf/declcd/schema/component"
)

release: component.#HelmRelease & {
	name:      "test"
	namespace: "test"
	chart: {
		name:    "test"
		repoURL: "http://test"
		version: "test"
	}
	values: {
		autoscaling: enabled: true
	}
	crds: {
		allowUpgrade: true
	}
}
