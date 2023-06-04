package prometheus

import "k8s.io/api/apps/v1"

_deployment: v1.#Deployment & {
	apiVersion: "v1"
	kind:       "Deployment"
	metadata: name: "mydeployment"
	spec: {
		replicas: 1
		template: {
			spec: {
				containers: [{
					name:  "nginx"
					image: "nginx:1.14.2"
					ports: [{
						containerPort: 80
					}]
				},
				]
			}
		}
	}
}
