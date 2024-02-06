package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Service(controllerName string, labels map[string]string, ns string) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []v1.ServicePort{
				{
					Name:       "http",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}
}
