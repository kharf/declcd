package v1

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Project(name string, labels map[string]string, annotations map[string]string, ns string, spec GitOpsProjectSpec) *GitOpsProject {
	return &GitOpsProject{
		TypeMeta: v1.TypeMeta{
			Kind:       "GitOpsProject",
			APIVersion: GroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}
}
