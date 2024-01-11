package install

import (
	"context"

	v1 "github.com/kharf/declcd/api/v1"
	"github.com/kharf/declcd/pkg/kube"
	"github.com/kharf/declcd/pkg/secret"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ControllerNamespace = "declcd-system"
	ControllerName      = "gitops-controller"
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
	kubeClient  *kube.DynamicClient
	projectRoot string
}

func NewAction(kubeClient *kube.DynamicClient, projectRoot string) Action {
	return Action{
		kubeClient:  kubeClient,
		projectRoot: projectRoot,
	}
}

func (act Action) Install(ctx context.Context, opts ...option) error {
	instOpts := options{
		namespace: ControllerNamespace,
	}
	for _, o := range opts {
		o.Apply(&instOpts)
	}
	labels := map[string]string{
		"declcd/component": ControllerName,
	}
	suspend := false
	objects := []client.Object{
		v1.CRD(labels),
		v1.Namespace(instOpts.namespace, labels),
		v1.ServiceAccount(ControllerName, labels, instOpts.namespace),
		v1.LeaderRole(instOpts.namespace, labels),
		v1.LeaderRoleBinding(ControllerName, labels, instOpts.namespace),
		v1.ClusterRole(ControllerName, labels),
		v1.ClusterRoleBinding(ControllerName, labels, instOpts.namespace),
		v1.StatefulSet(ControllerName, labels, instOpts.namespace),
	}
	for _, o := range objects {
		err := act.install(ctx, o, ControllerName)
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
	if err := secret.NewManager(act.projectRoot, instOpts.namespace, act.kubeClient).CreateKeyIfNotExists(ctx, ControllerName); err != nil {
		return err
	}
	if err := act.install(ctx, project, ControllerName); err != nil {
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
