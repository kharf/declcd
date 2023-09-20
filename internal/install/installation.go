package install

import (
	"context"

	"github.com/kharf/declcd/pkg/kube"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type options struct {
	namespace string
	image     string
}

type option interface {
	Apply(opts *options)
}

type Namespace string

var _ option = (*Namespace)(nil)

func (ns Namespace) Apply(opts *options) {
	opts.namespace = string(ns)
}

type Image string

var _ option = (*Image)(nil)

func (image Image) Apply(opts *options) {
	opts.image = string(image)
}

type action struct {
	kubeClient *kube.Client
}

func NewAction(kubeClient *kube.Client) action {
	return action{
		kubeClient: kubeClient,
	}
}

func (act action) Install(ctx context.Context, opts ...option) error {
	instOpts := options{}
	for _, o := range opts {
		o.Apply(&instOpts)
	}

	objects := []client.Object{
		&v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: instOpts.namespace,
			},
		},
		&v1.PersistentVolumeClaim{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "declcd",
				Namespace: instOpts.namespace,
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
		deployment(instOpts),
	}

	for _, o := range objects {
		err := act.install(ctx, o)
		if err != nil {
			return err
		}
	}

	return nil
}

func (act action) install(ctx context.Context, obj client.Object) error {
	pvcUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	if err := act.kubeClient.Apply(ctx, &unstructured.Unstructured{Object: pvcUnstructured}); err != nil {
		return err
	}

	return nil
}

func deployment(instOpts options) *appsv1.Deployment {
	controllerName := "declcd-controller"
	replicas := int32(1)
	labels := map[string]string{
		"app": controllerName,
	}
	nonRoot := true
	allowPriviligeEscalation := false
	// TODO: volume mount of pvc
	// TODO: make it a statefulset for multicluster sharding
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerName,
			Namespace: instOpts.namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					SecurityContext: &v1.PodSecurityContext{
						RunAsNonRoot: &nonRoot,
					},
					Containers: []v1.Container{
						{
							Name:    controllerName,
							Image:   instOpts.image,
							Command: []string{"/manager"},
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
									v1.ResourceCPU:    resource.MustParse("500m"),
									v1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("10m"),
									v1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}
