package prometheus

import (
	"github.com/kharf/declcd/api/v1"
)

_release: v1.#HelmRelease & {
	name:      "prometheus"
	namespace: #namespace.metadata.name
	chart: {
		name:    "kube-prometheus-stack"
		repoURL: "oci://ghcr.io/prometheus-community/charts"
		version: "56.0.1"
	}
	values: {
		coreDns: enabled: true
		kubeDns: enabled: false
		prometheus: prometheusSpec: {
			serviceMonitorSelectorNilUsesHelmValues: false
		}
	}
}
