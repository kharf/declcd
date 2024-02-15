package prometheus

import "github.com/kharf/declcd/api/v1"

v1.#Component & {
	prometheus: {
		manifests: [
			#namespace,
			_goDashboardConfigMap,
			_declcdDashboardConfigMap,
		]
		helmReleases: [
			_release,
		]
	}
}
