// Code generated by cue get go. DO NOT EDIT.

//cue:generate cue get go github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1

package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

#ThanosRulerKind:    "ThanosRuler"
#ThanosRulerName:    "thanosrulers"
#ThanosRulerKindKey: "thanosrulers"

// ThanosRuler defines a ThanosRuler deployment.
#ThanosRuler: {
	metav1.#TypeMeta
	metadata?: metav1.#ObjectMeta @go(ObjectMeta)

	// Specification of the desired behavior of the ThanosRuler cluster. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	spec: #ThanosRulerSpec @go(Spec)

	// Most recent observed status of the ThanosRuler cluster. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	status?: #ThanosRulerStatus @go(Status)
}

// ThanosRulerList is a list of ThanosRulers.
// +k8s:openapi-gen=true
#ThanosRulerList: {
	metav1.#TypeMeta

	// Standard list metadata
	// More info: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#metadata
	metadata?: metav1.#ListMeta @go(ListMeta)

	// List of Prometheuses
	items: [...null | #ThanosRuler] @go(Items,[]*ThanosRuler)
}

// ThanosRulerSpec is a specification of the desired behavior of the ThanosRuler. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
#ThanosRulerSpec: {
	// Version of Thanos to be deployed.
	version?: string @go(Version)

	// PodMetadata configures labels and annotations which are propagated to the ThanosRuler pods.
	//
	// The following items are reserved and cannot be overridden:
	// * "app.kubernetes.io/name" label, set to "thanos-ruler".
	// * "app.kubernetes.io/managed-by" label, set to "prometheus-operator".
	// * "app.kubernetes.io/instance" label, set to the name of the ThanosRuler instance.
	// * "thanos-ruler" label, set to the name of the ThanosRuler instance.
	// * "kubectl.kubernetes.io/default-container" annotation, set to "thanos-ruler".
	podMetadata?: null | #EmbeddedObjectMetadata @go(PodMetadata,*EmbeddedObjectMetadata)

	// Thanos container image URL.
	image?: string @go(Image)

	// When a ThanosRuler deployment is paused, no actions except for deletion
	// will be performed on the underlying objects.
	paused?: bool @go(Paused)

	// Number of thanos ruler instances to deploy.
	replicas?: null | int32 @go(Replicas,*int32)

	// Define which Nodes the Pods are scheduled on.
	nodeSelector?: {[string]: string} @go(NodeSelector,map[string]string)

	// Priority class assigned to the Pods
	priorityClassName?: string @go(PriorityClassName)

	// ServiceAccountName is the name of the ServiceAccount to use to run the
	// Thanos Ruler Pods.
	serviceAccountName?: string @go(ServiceAccountName)

	// Storage spec to specify how storage shall be used.
	storage?: null | #StorageSpec @go(Storage,*StorageSpec)

	// ObjectStorageConfigFile specifies the path of the object storage configuration file.
	// When used alongside with ObjectStorageConfig, ObjectStorageConfigFile takes precedence.
	objectStorageConfigFile?: null | string @go(ObjectStorageConfigFile,*string)

	// ListenLocal makes the Thanos ruler listen on loopback, so that it
	// does not bind against the Pod IP.
	listenLocal?: bool @go(ListenLocal)

	// QueryEndpoints defines Thanos querier endpoints from which to query metrics.
	// Maps to the --query flag of thanos ruler.
	queryEndpoints?: [...string] @go(QueryEndpoints,[]string)

	// Define URLs to send alerts to Alertmanager.  For Thanos v0.10.0 and higher,
	// AlertManagersConfig should be used instead.  Note: this field will be ignored
	// if AlertManagersConfig is specified.
	// Maps to the `alertmanagers.url` arg.
	alertmanagersUrl?: [...string] @go(AlertManagersURL,[]string)

	// A label selector to select which PrometheusRules to mount for alerting and
	// recording.
	ruleSelector?: null | metav1.#LabelSelector @go(RuleSelector,*metav1.LabelSelector)

	// Namespaces to be selected for Rules discovery. If unspecified, only
	// the same namespace as the ThanosRuler object is in is used.
	ruleNamespaceSelector?: null | metav1.#LabelSelector @go(RuleNamespaceSelector,*metav1.LabelSelector)

	// EnforcedNamespaceLabel enforces adding a namespace label of origin for each alert
	// and metric that is user created. The label value will always be the namespace of the object that is
	// being created.
	enforcedNamespaceLabel?: string @go(EnforcedNamespaceLabel)

	// List of references to PrometheusRule objects
	// to be excluded from enforcing a namespace label of origin.
	// Applies only if enforcedNamespaceLabel set to true.
	excludedFromEnforcement?: [...#ObjectReference] @go(ExcludedFromEnforcement,[]ObjectReference)

	// PrometheusRulesExcludedFromEnforce - list of Prometheus rules to be excluded from enforcing
	// of adding namespace labels. Works only if enforcedNamespaceLabel set to true.
	// Make sure both ruleNamespace and ruleName are set for each pair
	// Deprecated: use excludedFromEnforcement instead.
	prometheusRulesExcludedFromEnforce?: [...#PrometheusRuleExcludeConfig] @go(PrometheusRulesExcludedFromEnforce,[]PrometheusRuleExcludeConfig)

	// Log level for ThanosRuler to be configured with.
	// +kubebuilder:validation:Enum="";debug;info;warn;error
	logLevel?: string @go(LogLevel)

	// Log format for ThanosRuler to be configured with.
	// +kubebuilder:validation:Enum="";logfmt;json
	logFormat?: string @go(LogFormat)

	// Port name used for the pods and governing service.
	// Defaults to `web`.
	// +kubebuilder:default:="web"
	portName?: string @go(PortName)

	// Interval between consecutive evaluations.
	// +kubebuilder:default:="15s"
	evaluationInterval?: #Duration @go(EvaluationInterval)

	// Time duration ThanosRuler shall retain data for. Default is '24h',
	// and must match the regular expression `[0-9]+(ms|s|m|h|d|w|y)` (milliseconds seconds minutes hours days weeks years).
	// +kubebuilder:default:="24h"
	retention?: #Duration @go(Retention)

	// TracingConfig specifies the path of the tracing configuration file.
	// When used alongside with TracingConfig, TracingConfigFile takes precedence.
	tracingConfigFile?: string @go(TracingConfigFile)

	// Labels configure the external label pairs to ThanosRuler. A default replica label
	// `thanos_ruler_replica` will be always added  as a label with the value of the pod's name and it will be dropped in the alerts.
	labels?: {[string]: string} @go(Labels,map[string]string)

	// AlertDropLabels configure the label names which should be dropped in ThanosRuler alerts.
	// The replica label `thanos_ruler_replica` will always be dropped in alerts.
	alertDropLabels?: [...string] @go(AlertDropLabels,[]string)

	// The external URL the Thanos Ruler instances will be available under. This is
	// necessary to generate correct URLs. This is necessary if Thanos Ruler is not
	// served from root of a DNS name.
	externalPrefix?: string @go(ExternalPrefix)

	// The route prefix ThanosRuler registers HTTP handlers for. This allows thanos UI to be served on a sub-path.
	routePrefix?: string @go(RoutePrefix)

	// GRPCServerTLSConfig configures the gRPC server from which Thanos Querier reads
	// recorded rule data.
	// Note: Currently only the CAFile, CertFile, and KeyFile fields are supported.
	// Maps to the '--grpc-server-tls-*' CLI args.
	grpcServerTlsConfig?: null | #TLSConfig @go(GRPCServerTLSConfig,*TLSConfig)

	// The external Query URL the Thanos Ruler will set in the 'Source' field
	// of all alerts.
	// Maps to the '--alert.query-url' CLI arg.
	alertQueryUrl?: string @go(AlertQueryURL)

	// Minimum number of seconds for which a newly created pod should be ready
	// without any of its container crashing for it to be considered available.
	// Defaults to 0 (pod will be considered available as soon as it is ready)
	// This is an alpha field from kubernetes 1.22 until 1.24 which requires enabling the StatefulSetMinReadySeconds feature gate.
	// +optional
	minReadySeconds?: null | uint32 @go(MinReadySeconds,*uint32)

	// AlertRelabelConfigFile specifies the path of the alert relabeling configuration file.
	// When used alongside with AlertRelabelConfigs, alertRelabelConfigFile takes precedence.
	alertRelabelConfigFile?: null | string @go(AlertRelabelConfigFile,*string)

	// Pods' hostAliases configuration
	// +listType=map
	// +listMapKey=ip
	hostAliases?: [...#HostAlias] @go(HostAliases,[]HostAlias)

	// AdditionalArgs allows setting additional arguments for the ThanosRuler container.
	// It is intended for e.g. activating hidden flags which are not supported by
	// the dedicated configuration options yet. The arguments are passed as-is to the
	// ThanosRuler container which may cause issues if they are invalid or not supported
	// by the given ThanosRuler version.
	// In case of an argument conflict (e.g. an argument which is already set by the
	// operator itself) or when providing an invalid argument the reconciliation will
	// fail and an error will be logged.
	additionalArgs?: [...#Argument] @go(AdditionalArgs,[]Argument)
}

// ThanosRulerStatus is the most recent observed status of the ThanosRuler. Read-only.
// More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
#ThanosRulerStatus: {
	// Represents whether any actions on the underlying managed objects are
	// being performed. Only delete actions will be performed.
	paused: bool @go(Paused)

	// Total number of non-terminated pods targeted by this ThanosRuler deployment
	// (their labels match the selector).
	replicas: int32 @go(Replicas)

	// Total number of non-terminated pods targeted by this ThanosRuler deployment
	// that have the desired version spec.
	updatedReplicas: int32 @go(UpdatedReplicas)

	// Total number of available pods (ready for at least minReadySeconds)
	// targeted by this ThanosRuler deployment.
	availableReplicas: int32 @go(AvailableReplicas)

	// Total number of unavailable pods targeted by this ThanosRuler deployment.
	unavailableReplicas: int32 @go(UnavailableReplicas)

	// The current state of the Alertmanager object.
	// +listType=map
	// +listMapKey=type
	// +optional
	conditions?: [...#Condition] @go(Conditions,[]Condition)
}
