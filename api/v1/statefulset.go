package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func StatefulSet(controllerName string, labels map[string]string, ns string) *appsv1.StatefulSet {
	replicas := int32(1)
	nonRoot := true
	allowPriviligeEscalation := false
	pvcName := "declcd"
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerName,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &replicas,
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: pvcName,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse("20Mi"),
							},
						},
					},
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					ServiceAccountName: controllerName,
					SecurityContext: &v1.PodSecurityContext{
						RunAsNonRoot: &nonRoot,
					},
					Volumes: []v1.Volume{
						{
							Name: "podinfo",
							VolumeSource: v1.VolumeSource{
								DownwardAPI: &v1.DownwardAPIVolumeSource{
									Items: []v1.DownwardAPIVolumeFile{
										{
											Path: "namespace",
											FieldRef: &v1.ObjectFieldSelector{
												FieldPath: "metadata.namespace",
											},
										},
									},
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:    controllerName,
							Image:   "ghcr.io/kharf/declcd:latest",
							Command: []string{"/controller"},
							Args:    []string{"--leader-elect"},
							SecurityContext: &v1.SecurityContext{
								AllowPrivilegeEscalation: &allowPriviligeEscalation,
								Capabilities: &v1.Capabilities{
									Drop: []v1.Capability{
										v1.Capability("ALL"),
									},
								},
							},
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("1.5Gi"),
								},
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("500m"),
									v1.ResourceMemory: resource.MustParse("1.5Gi"),
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      pvcName,
									MountPath: "/inventory",
								},
								{
									Name:      "podinfo",
									MountPath: "/podinfo",
								},
							},
						},
					},
				},
			},
		},
	}
}
