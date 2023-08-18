package subcomponent

import (
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
	"k8s.io/api/apps/v1"
)

_deployment: v1.#Deployment & {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      "mysubcomponent"
		namespace: prometheus.#namespace.metadata.name
	}
	spec: {
		replicas: 1
		selector: matchLabels: app: _deployment.metadata.name
		template: {
			metadata: labels: app: _deployment.metadata.name
			spec: {
				containers: [{
					name:  "subcomponent"
					image: "subcomponent:1.14.2"
					ports: [{
						containerPort: 80
					}]
				},
				]
			}
		}
	}
}
