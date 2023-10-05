package v1

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ClusterRole(controllerName string, labels map[string]string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRole",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   controllerName,
			Labels: labels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"gitops.declcd.io"},
				Resources: []string{"gitopsprojects"},
				Verbs: []string{
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"gitops.declcd.io"},
				Resources: []string{"gitopsprojects/status"},
				Verbs: []string{
					"get",
					"patch",
					"update",
				},
			},
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs: []string{
					"*",
				},
			},
		},
	}
}

func ClusterRoleBinding(controllerName string, labels map[string]string, ns string) *rbacv1.ClusterRoleBinding {
	group := "rbac.authorization.k8s.io"
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fmt.Sprintf("%s/v1", group),
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerName,
			Namespace: ns,
			Labels:    labels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: group,
			Kind:     "ClusterRole",
			Name:     controllerName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      controllerName,
				Namespace: ns,
			},
		},
	}
}
