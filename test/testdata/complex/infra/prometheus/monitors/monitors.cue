package monitors

import (
	"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

_declcdMonitor: v1.#ServiceMonitor & {
	apiVersion: "monitoring.coreos.com/v1"
	kind:       "ServiceMonitor"
	metadata: {
		labels: {
			"declcd/control-plane": "gitops-controller"
		}
		name:      "gitops-controller"
		namespace: "declcd-system"
	}
	spec: {
		endpoints: [{
			port: "http"
		}]
		selector: matchLabels: "declcd/control-plane": "gitops-controller"
	}
}
