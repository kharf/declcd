package install

import (
	"context"

	v1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type options struct {
	namespace string
	url       string
	branch    string
	stage     string
	interval  int
}

type option interface {
	Apply(opts *options)
}

type Namespace string

var _ option = (*Namespace)(nil)

func (ns Namespace) Apply(opts *options) {
	opts.namespace = string(ns)
}

type URL string

var _ option = (*URL)(nil)

func (url URL) Apply(opts *options) {
	opts.url = string(url)
}

type Branch string

var _ option = (*Branch)(nil)

func (branch Branch) Apply(opts *options) {
	opts.branch = string(branch)
}

type Stage string

var _ option = (*Stage)(nil)

func (stage Stage) Apply(opts *options) {
	opts.stage = string(stage)
}

type Interval int

var _ option = (*Interval)(nil)

func (interval Interval) Apply(opts *options) {
	opts.interval = int(interval)
}

type Action struct {
	kubeClient *kube.Client
}

func NewAction(kubeClient *kube.Client) Action {
	return Action{
		kubeClient: kubeClient,
	}
}

func (act Action) Install(ctx context.Context, opts ...option) error {
	instOpts := options{
		namespace: "default",
	}
	for _, o := range opts {
		o.Apply(&instOpts)
	}

	controllerName := "gitops-controller"
	labels := map[string]string{
		"declcd/component": controllerName,
	}
	suspend := false
	objects := []client.Object{
		v1.CRD(labels),
		v1.Namespace(instOpts.namespace, labels),
		v1.ServiceAccount(controllerName, labels, instOpts.namespace),
		v1.LeaderRole(instOpts.namespace, labels),
		v1.LeaderRoleBinding(controllerName, labels, instOpts.namespace),
		v1.ClusterRole(controllerName, labels),
		v1.ClusterRoleBinding(controllerName, labels, instOpts.namespace),
		v1.StatefulSet(controllerName, labels, instOpts.namespace),
	}

	for _, o := range objects {
		err := act.install(ctx, o, controllerName)
		if err != nil {
			return err
		}
	}

	project := v1.Project(instOpts.stage, labels, instOpts.namespace, v1.GitOpsProjectSpec{
		URL:                 instOpts.url,
		Branch:              instOpts.branch,
		Stage:               instOpts.stage,
		PullIntervalSeconds: instOpts.interval,
		Suspend:             &suspend,
	})

	// clear cache because we just introduced a new crd
	if err := act.kubeClient.Invalidate(); err != nil {
		return err
	}

	err := act.install(ctx, project, controllerName)
	if err != nil {
		return err
	}

	return nil
}

func (act Action) install(ctx context.Context, obj client.Object, fieldManager string) error {
	unstrObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}

	unstr := &unstructured.Unstructured{Object: unstrObj}
	if err := act.kubeClient.Apply(ctx, unstr, fieldManager); err != nil {
		return err
	}

	return nil
}
