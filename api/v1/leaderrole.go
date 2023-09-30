package v1

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func LeaderRole(ns string, labels map[string]string) *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leader-election",
			Namespace: ns,
			Labels:    labels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs: []string{
					"get",
					"create",
					"update",
				},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs: []string{
					"create",
					"patch",
				},
			},
		},
	}
}

func LeaderRoleBinding(controllerName string, labels map[string]string, ns string) *rbacv1.RoleBinding {
	group := "rbac.authorization.k8s.io"
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fmt.Sprintf("%s/v1", group),
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leader-election",
			Namespace: ns,
			Labels:    labels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: group,
			Kind:     "Role",
			Name:     "leader-election",
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
