package subcomponent

import (
	"github.com/kharf/declcd/test/testdata/simple/infra/prometheus"
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/apps/v1"
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
				securityContext: {
					runAsNonRoot:        true
					fsGroup:             65532
					fsGroupChangePolicy: "OnRootMismatch"
				}
				containers: [
					{
						name:  "subcomponent"
						image: "subcomponent:1.14.2"
						ports: [{
							name:          "http"
							containerPort: 80
						}]
					},
					{
						name:  "sidecar"
						image: "sidecar:1.14.2"
						ports: [{
							name:          "http"
							containerPort: 80
						}]
					},
				]
			}
		}
	}
}

_anotherDeployment: v1.#Deployment & {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      "anothersubcomponent"
		namespace: prometheus.#namespace.metadata.name
	}
	spec: {
		replicas: 1 @ignore(conflict)
		selector: matchLabels: app: _deployment.metadata.name
		template: {
			metadata: labels: app: _deployment.metadata.name
			spec: {
				securityContext: {
					runAsNonRoot:        true  @ignore(conflict)
					fsGroup:             65532 @ignore(conflict)
					fsGroupChangePolicy: "OnRootMismatch"
				}
				containers: [
					{
						name:  "subcomponent"
						image: "subcomponent:1.14.2"
						ports: [{
							name:          "http"
							containerPort: 80
						}]
					},
				]
			}
		}
	}
}
