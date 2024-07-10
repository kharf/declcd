package prometheus

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/test/testdata/simple/infra/linkerd"
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/apps/v1"
)

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
		linkerd.ns.id,
	]
	name:      "{{.Name}}"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "{{.RepoURL}}"
		version: "1.0.0"
	}

	crds: {
		allowUpgrade: true
	}

	values: {
		autoscaling: enabled: true
	}

	patches: [
		v1.#Deployment & {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {
				name:      "{{.Name}}"
				namespace: #namespace.metadata.name
			}
			spec: {
				replicas: 5 @ignore(conflict)
				template: {
					spec: {
						containers: [
							{
								name:  "prometheus"
								image: "prometheus:1.14.2"
								ports: [{
									containerPort: 80
								}]
							},
							{
								name:  "sidecar"
								image: "sidecar:1.14.2" @ignore(conflict) // attributes in lists are not supported
								ports: [{
									containerPort: 80
								}]
							},
						] @ignore(conflict)
					}
				}
			}
		},
	]
}
