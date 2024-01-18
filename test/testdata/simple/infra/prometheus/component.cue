package prometheus

import (
	v1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
)

v1.#Component & {
	prometheus: {
		dependencies: [
			linkerd.linkerd.id,
		]
		manifests: [
			#namespace,
			_secret,
		]
		helmReleases: [
			_release,
		]
	}
}
