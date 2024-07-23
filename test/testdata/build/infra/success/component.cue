package success

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

#deployment: v1.#Deployment & {
	apiVersion: string | *"apps/v1"
	kind:       "Deployment"
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
		#deployment & {
			metadata: {
				name:      "test"
				namespace: ns.content.metadata.name
			}
			spec: {
				replicas: 1 @ignore(conflict)
			}
		},
		#deployment & {
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

crd: component.#Manifest & {
	content: {
		apiVersion: "apiextensions.k8s.io/v1"
		kind:       "CustomResourceDefinition"
		metadata: {
			annotations: "controller-gen.kubebuilder.io/version": "v0.15.0"
			name: "gitopsprojects.gitops.declcd.io"
		}
		spec: {
			group: "gitops.declcd.io"
			names: {
				kind:     "GitOpsProject"
				listKind: "GitOpsProjectList"
				plural:   "gitopsprojects"
				singular: "gitopsproject"
			}
			scope: "Namespaced"
			versions: [{
				name: "v1beta1"
				schema: openAPIV3Schema: {
					description: "GitOpsProject is the Schema for the gitopsprojects API"
					properties: {
						apiVersion: {
							description: """
	APIVersion defines the versioned schema of this representation of an object.
	Servers should convert recognized schemas to the latest internal value, and
	may reject unrecognized values.
	More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	"""
							type: "string"
						}
						kind: {
							description: """
	Kind is a string value representing the REST resource this object represents.
	Servers may infer this from the endpoint the client submits requests to.
	Cannot be updated.
	In CamelCase.
	More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	"""
							type: "string"
						}
						metadata: type: "object"
						spec: {
							description: "GitOpsProjectSpec defines the desired state of GitOpsProject"
							properties: {
								branch: {
									description: "The branch of the gitops repository holding the declcd configuration."
									minLength:   1
									type:        "string"
								}
								pullIntervalSeconds: {
									description: "This defines how often declcd will try to fetch changes from the gitops repository."
									minimum:     5
									type:        "integer"
								}
								serviceAccountName: type: "string"
								suspend: {
									description: """
	This flag tells the controller to suspend subsequent executions, it does
	not apply to already started executions.  Defaults to false.
	"""
									type: "boolean"
								}
								url: {
									description: "The url to the gitops repository."
									minLength:   1
									type:        "string"
								}
							}
							required: [
								"branch",
								"pullIntervalSeconds",
								"url",
							]
							type: "object"
						}
						status: {
							description: "GitOpsProjectStatus defines the observed state of GitOpsProject"
							properties: {
								conditions: {
									items: {
										description: ""
										properties: {
											lastTransitionTime: {
												description: """
	lastTransitionTime is the last time the condition transitioned from one status to another.
	This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	"""
												format: "date-time"
												type:   "string"
											}
											message: {
												description: """
	message is a human readable message indicating details about the transition.
	This may be an empty string.
	"""
												maxLength: 32768
												type:      "string"
											}
											observedGeneration: {
												description: """
	observedGeneration represents the .metadata.generation that the condition was set based upon.
	For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	with respect to the current state of the instance.
	"""
												format:  "int64"
												minimum: 0
												type:    "integer"
											}
											reason: {
												description: """
	reason contains a programmatic identifier indicating the reason for the condition's last transition.
	Producers of specific condition types may define expected values and meanings for this field,
	and whether the values are considered a guaranteed API.
	The value should be a CamelCase string.
	This field may not be empty.
	"""
												maxLength: 1024
												minLength: 1
												pattern:   "^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$"
												type:      "string"
											}
											status: {
												description: "status of the condition, one of True, False, Unknown."
												enum: [
													"True",
													"False",
													"Unknown",
												]
												type: "string"
											}
											type: {
												description: ""
												maxLength:   316
												pattern:     "^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$"
												type:        "string"
											}
										}
										required: [
											"lastTransitionTime",
											"message",
											"reason",
											"status",
											"type",
										]
										type: "object"
									}
									type: "array"
								}
								revision: {
									properties: {
										commitHash: type: "string"
										reconcileTime: {
											format: "date-time"
											type:   "string"
										}
									}
									type: "object"
								}
							}
							type: "object"
						}
					}
					type: "object"
				}
				served:  true
				storage: true
				subresources: status: {}
			}]
		}
	}
}
