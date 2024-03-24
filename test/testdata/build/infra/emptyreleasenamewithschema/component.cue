package emptyreleasenamewithschema

import (
	"github.com/kharf/declcd/schema@v0"
)

release: schema.#HelmRelease & {
	name: ""
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
