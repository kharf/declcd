package pyroscope

import (
	"github.com/kharf/declcd/api/v1"
	"github.com/kharf/decldc-test-repo/infra/prometheus"
)

_release: v1.#HelmRelease & {
	name:      "pyroscope"
	namespace: prometheus.#namespace.metadata.name
	chart: {
		name:    "pyroscope"
		repoURL: "https://grafana.github.io/helm-charts"
		version: "1.4.0"
	}
	values: {
	}
}
