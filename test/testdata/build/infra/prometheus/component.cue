package prometheus

import (
	"github.com/kharf/declcd/schema/component"
	"github.com/kharf/declcd/schema/workloadidentity"
	corev1 "github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/apps/v1"
)

#namespace: corev1.#Namespace & {
	@ignore(conflict)

	apiVersion: string | *"v1" @ignore(conflict)
	kind:       "Namespace"
	metadata: {
		name: "prometheus" @ignore(conflict)
	}
}

ns: component.#Manifest & {
	content: #namespace
}

#secret: corev1.#Secret & {
	apiVersion: string | *"v1"
	kind:       string | *"Secret"
	metadata: {
		name:      "secret"
		namespace: #namespace.metadata.name
	} | {
		name:      "default-secret-name"
		namespace: "default-secret-namespace"
	}
}

secret: component.#Manifest & {
	dependencies: [
		ns.id,
	]
	content: #secret & {
		metadata: {
			name: "secret"
		}
		data: {
			foo: 'bar' @ignore(conflict)
		}
	}
}

_deployment: v1.#Deployment & {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      "prometheus"
		namespace: ns.content.metadata.name
	}
	spec: {
		replicas: 1 @ignore(conflict)
		selector: matchLabels: app: _deployment.metadata.name
		template: {
			metadata: labels: app: _deployment.metadata.name
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
}

deployment: component.#Manifest & {
	dependencies: [
		ns.id,
	]
	content: _deployment
}

role: component.#Manifest & {
	dependencies: [ns.id]
	content: {
		apiVersion: "rbac.authorization.k8s.io/v1"
		kind:       "Role"
		metadata: {
			name:      "prometheus"
			namespace: ns.content.metadata.name
		}
		rules: [
			{
				apiGroups: ["coordination.k8s.io"]
				resources: ["leases"]
				verbs: [
					"get",
					"create",
					"update",
				]
			},
			{
				apiGroups: [""]
				resources: ["events"]
				verbs: [
					"create",
					"patch",
				]
			},
		]
	}
}

release: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
	}

	patches: [
		v1.#Deployment & {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {
				name:      "test"
				namespace: ns.content.metadata.name
			}
			spec: {
				replicas: 1 @ignore(conflict)
			}
		},
		v1.#Deployment & {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {
				name:      "hello"
				namespace: ns.content.metadata.name
			}
			spec: {
				replicas: 2 @ignore(conflict)
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

	values: {
		autoscaling: enabled: true
	}
}

releaseSecretRef: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test-secret-ref"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
		auth: secretRef: {
			name:      "secret"
			namespace: "namespace"
		}
	}
}

releaseWorkloadIdentity: component.#HelmRelease & {
	dependencies: [
		ns.id,
	]
	name:      "test-workload-identity"
	namespace: #namespace.metadata.name
	chart: {
		name:    "test"
		repoURL: "oci://test"
		version: "test"
		auth:    workloadidentity.#GCP
	}
	values: {
		autoscaling: enabled: true
	}
}
