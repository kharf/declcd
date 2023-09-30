package v1

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CRD(labels map[string]string) *apiextensionsv1.CustomResourceDefinition {
	minimumPullInterval := float64(5)
	required := int64(1)
	conditionMessageMaxLength := int64(32768)
	observedGenerationMinimum := float64(0)
	reasonMaxLength := int64(1024)
	reasonMinLength := int64(1)
	typeMaxLength := int64(316)
	return &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "gitopsprojects.gitops.declcd.io",
			Labels: labels,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: GroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:       "GitOpsProject",
				ListKind:   "GitOpsProjectList",
				Plural:     "gitopsprojects",
				Singular:   "gitopsproject",
				ShortNames: []string{"gp", "gop"},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: "v1",
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:        "object",
							Description: "GitOpsProject is the Schema for the gitopsprojects API",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"apiVersion": {
									Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
									Type:        "string",
								},
								"kind": {
									Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
									Type:        "string",
								},
								"metadata": {
									Type: "object",
								},
								"spec": {
									Description: "GitOpsProjectSpec defines the desired state of GitOpsProject",
									Type:        "object",
									Required:    []string{"pullIntervalSeconds", "url"},
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"pullIntervalSeconds": {
											Description: "This defines how often decl will try to fetch changes from the gitops repository",
											Minimum:     &minimumPullInterval,
											Type:        "integer",
										},
										"suspend": {
											Description: "This flag tells the controller to suspend subsequent executions, it does not apply to already started executions.  Defaults to false",
											Minimum:     &minimumPullInterval,
											Type:        "boolean",
										},
										"url": {
											Description: "The url to the gitops repository",
											MinLength:   &required,
											Type:        "string",
										},
										"branch": {
											Description: "The branch of the gitops repository holding the declcd configuration",
											MinLength:   &required,
											Type:        "string",
										},
										"stage": {
											Description: "The stage of the declcd configuration",
											MinLength:   &required,
											Type:        "string",
										},
									},
								},
								"status": {
									Description: "GitOpsProjectSpec defines the desired state of GitOpsProject",
									Type:        "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"conditions": {
											Type: "array",
											Items: &apiextensionsv1.JSONSchemaPropsOrArray{
												Schema: &apiextensionsv1.JSONSchemaProps{
													Type:        "object",
													Description: "Condition contains details for one aspect of the current state of this API Resource. --- This struct is intended for direct use as an array at the field path .status.conditions.  For example, \n type FooStatus struct{ // Represents the observations of a foo's current state. // Known .status.conditions.type are: \"Available\", \"Progressing\", and \"Degraded\" // +patchMergeKey=type // +patchStrategy=merge // +listType=map // +listMapKey=type Conditions []metav1.Condition json:\"conditions,omitempty\" patchStrategy:\"merge\" patchMergeKey:\"type\" protobuf:\"bytes,1,rep,name=conditions\" // other fields }",
													Required:    []string{"lastTransitionTime", "message", "reason", "status", "type"},
													Properties: map[string]apiextensionsv1.JSONSchemaProps{
														"lastTransitionTime": {
															Description: "lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable",
															Format:      "date-time",
															Type:        "string",
														},
														"message": {
															Description: "message is a human readable message indicating details about the transition. This may be an empty string",
															MaxLength:   &conditionMessageMaxLength,
															Type:        "string",
														},
														"observedGeneration": {
															Description: "observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance",
															Format:      "int64",
															Minimum:     &observedGenerationMinimum,
															Type:        "integer",
														},
														"reason": {
															Description: "reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty",
															Pattern:     "^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$",
															MaxLength:   &reasonMaxLength,
															MinLength:   &reasonMinLength,
															Type:        "string",
														},
														"status": {
															Description: "status of the condition, one of True, False, Unknown",
															Enum: []apiextensionsv1.JSON{
																{
																	Raw: []byte(fmt.Sprintf("\"%s\"", apiextensionsv1.ConditionTrue)),
																},
																{
																	Raw: []byte(fmt.Sprintf("\"%s\"", apiextensionsv1.ConditionFalse)),
																},
																{
																	Raw: []byte("\"Unknown\""),
																},
															},
															Type: "string",
														},
														"type": {
															Description: "type of condition in CamelCase or in foo.example.com/CamelCase. --- Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be useful (see .node.status.conditions), the ability to deconflict is important. The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)",
															Pattern:     "^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$",
															MaxLength:   &typeMaxLength,
															Type:        "string",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
				},
			},
		},
	}
}
