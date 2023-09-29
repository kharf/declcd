/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOpsProjectSpec defines the desired state of GitOpsProject
type GitOpsProjectSpec struct {
	//+kubebuilder:validation:MinLength=1

	// The url to the gitops repository.
	URL string `json:"url"`

	//+kubebuilder:validation:Minimum=5

	// This defines how often decl will try to fetch changes from the gitops repository.
	PullIntervalSeconds int `json:"pullIntervalSeconds"`

	// This flag tells the controller to suspend subsequent executions, it does
	// not apply to already started executions.  Defaults to false.
	// +optional
	Suspend *bool `json:"suspend,omitempty"`
}

// GitOpsProjectStatus defines the observed state of GitOpsProject
type GitOpsProjectStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GitOpsProject is the Schema for the gitopsprojects API
type GitOpsProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsProjectSpec   `json:"spec,omitempty"`
	Status GitOpsProjectStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GitOpsProjectList contains a list of GitOpsProject
type GitOpsProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOpsProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOpsProject{}, &GitOpsProjectList{})
}
