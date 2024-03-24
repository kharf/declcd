package emptyreleasechartversionwithschema

import (
	"github.com/kharf/declcd/schema@v0"
)

release: schema.#HelmRelease & {
	name: "test"
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
