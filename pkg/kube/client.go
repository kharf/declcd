package kube

import (
	"context"
	"errors"
	"slices"
	"time"

	"helm.sh/helm/v3/pkg/action"
	helmKube "helm.sh/helm/v3/pkg/kube"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/resource"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

type InMemoryRESTClientGetter struct {
	Cfg        *rest.Config
	RestMapper meta.RESTMapper
}

var _ action.RESTClientGetter = (*InMemoryRESTClientGetter)(nil)

func (c *InMemoryRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.Cfg, nil
}

func (c *InMemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	client, err := discovery.NewDiscoveryClientForConfig(c.Cfg)
	return memory.NewMemCacheClient(client), err
}

func (c *InMemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return c.RestMapper, nil
}

func (c *InMemoryRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}

type applyOptions struct {
	dryRun bool
	force  bool
}

// ApplyOption is a specific configuration used for applying changes to an object.
type ApplyOption interface {
	Apply(opts *applyOptions)
}

// DryRun indicates that modifications should not be persisted.
type DryRun bool

func (dr DryRun) Apply(opts *applyOptions) {
	opts.dryRun = bool(dr)
}

// Force indicates that conflicts should not error.
type Force bool

func (f Force) Apply(opts *applyOptions) {
	opts.force = bool(f)
}

// Client connects to a Kubernetes cluster
// to create, read, update and delete manifests/objects.
type Client[T any] interface {
	// Apply applies changes to an object through a Server-Side Apply
	// and takes the ownership of this object.
	// The object is created when it does not exist.
	// It errors on conflicts if force is set to false.
	Apply(ctx context.Context, obj *T, fieldManager string, opts ...ApplyOption) error
	// Update applies changes to an object.
	// The object is created when it does not exist.
	// It does not error on conflicts.
	Update(ctx context.Context, obj *T, fieldManager string, opts ...ApplyOption) error
	// Get retrieves the unstructured object from a Kubernetes cluster.
	Get(ctx context.Context, obj *T) (*T, error)
	// Delete removes the object from the Kubernetes cluster.
	Delete(ctx context.Context, obj *T) error
	// Returns the [meta.RESTMapper] associated with this client.
	RESTMapper() meta.RESTMapper
}

// DynamicClient connects to a Kubernetes cluster
// to create, read, update and delete unstructured manifests/objects.
type DynamicClient struct {
	dynamicClient *dynamic.DynamicClient
	restMapper    meta.RESTMapper
	invalidate    func()
}

var _ Client[unstructured.Unstructured] = (*DynamicClient)(nil)

// NewDynamicClient constructs a new DynamicClient,
// which connects to a Kubernetes cluster to create, read, update and delete unstructured manifests/objects.
func NewDynamicClient(config *rest.Config) (*DynamicClient, error) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	cacheClient := memory.NewMemCacheClient(discoveryClient)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cacheClient)
	return &DynamicClient{
		dynamicClient: dynClient,
		restMapper:    restMapper,
		invalidate:    restMapper.Reset,
	}, nil
}

// Invalidate resets the internally cached Discovery information and will
// cause the next mapping request to re-discover.
func (client *DynamicClient) Invalidate() error {
	client.invalidate()
	return nil
}

// Apply applies changes to an object through a Server-Side Apply
// and takes the ownership of this object.
// The object is created when it does not exist.
// It errors on conflicts if force is set to false.
func (client *DynamicClient) Apply(
	ctx context.Context,
	obj *unstructured.Unstructured,
	fieldManager string,
	opts ...ApplyOption,
) error {
	applyOptions := new(applyOptions)
	for _, opt := range opts {
		opt.Apply(applyOptions)
	}
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}
	createOptions := v1.ApplyOptions{
		FieldManager: fieldManager,
		Force:        applyOptions.force,
	}
	if applyOptions.dryRun {
		createOptions.DryRun = []string{"All"}
	}
	_, err = resourceInterface.Apply(ctx, obj.GetName(), obj, createOptions)
	if err != nil {
		return err
	}
	if !applyOptions.dryRun {
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, err = client.wait(
			timeoutCtx,
			obj.GetName(),
			v1.TypeMeta{
				Kind:       obj.GetKind(),
				APIVersion: obj.GetAPIVersion(),
			},
			resourceInterface,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// Update applies changes to an object.
// The object is created when it does not exist.
// It does not error on conflicts.
func (client *DynamicClient) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	fieldManager string,
	opts ...ApplyOption,
) error {
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}
	_, err = resourceInterface.Create(ctx, obj, v1.CreateOptions{FieldManager: fieldManager})
	if err != nil {
		if k8sErrors.ReasonForError(err) == v1.StatusReasonAlreadyExists {
			existingObj, err := resourceInterface.Get(
				ctx,
				obj.GetName(),
				v1.GetOptions{TypeMeta: v1.TypeMeta{
					Kind:       obj.GetKind(),
					APIVersion: obj.GetAPIVersion(),
				},
				},
			)
			if err != nil {
				return err
			}
			obj.SetResourceVersion(existingObj.GetResourceVersion())
			_, err = resourceInterface.Update(
				ctx,
				obj,
				v1.UpdateOptions{FieldManager: fieldManager},
			)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = client.wait(
		timeoutCtx,
		obj.GetName(),
		v1.TypeMeta{
			Kind:       obj.GetKind(),
			APIVersion: obj.GetAPIVersion(),
		},
		resourceInterface,
	)
	if err != nil {
		return err
	}
	return nil
}

func (client *DynamicClient) wait(
	ctx context.Context,
	name string,
	typeMeta v1.TypeMeta,
	resourceInterface dynamic.ResourceInterface,
) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	obj, err := resourceInterface.Get(ctx, name, v1.GetOptions{
		TypeMeta: typeMeta,
	})
	if err != nil {
		return false, err
	}
	if obj.GetKind() != "CustomResourceDefinition" {
		return true, nil
	}
	conditions := getConditions(obj)
	ok := slices.ContainsFunc(conditions, func(cond condition) bool {
		return cond.cType == string(apiextensionsv1.Established)
	})
	if ok {
		return true, nil
	}
	time.Sleep(1 * time.Second)
	return client.wait(ctx, name, typeMeta, resourceInterface)
}

type condition struct {
	cType string
}

func getConditions(obj *unstructured.Unstructured) []condition {
	conditions := make([]condition, 0, 2)
	status, ok := obj.Object["status"]
	if !ok {
		return conditions
	}
	statusMap, ok := status.(map[string]interface{})
	if !ok {
		return conditions
	}
	conditionsArr, ok := statusMap["conditions"].([]interface{})
	if !ok {
		return conditions
	}
	for _, c := range conditionsArr {
		cond, ok := c.(map[string]interface{})
		if !ok {
			return conditions
		}
		t, ok := cond["type"].(string)
		if !ok {
			return conditions
		}
		conditions = append(conditions, condition{cType: t})
	}
	return conditions
}

// Delete removes the unstructured object from a Kubernetes cluster.
// Following fields have to be set on obj:
// - GVK, Namespace, Name
func (client *DynamicClient) Delete(ctx context.Context, obj *unstructured.Unstructured) error {
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}
	if err := resourceInterface.Delete(ctx, obj.GetName(), v1.DeleteOptions{
		TypeMeta: v1.TypeMeta{
			Kind:       obj.GetKind(),
			APIVersion: obj.GetAPIVersion(),
		},
	}); err != nil {
		return err
	}
	return nil
}

var (
	ErrManifestNoMetadata = errors.New("Helm chart manifest has no metadata")
)

// Get retrieves the unstructured object from a Kubernetes cluster.
// Following fields have to be set on obj:
// - GVK, Name
func (client *DynamicClient) Get(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	namespace := ""
	metadata, ok := obj.Object["metadata"].(map[string]interface{})
	if !ok {
		return nil, ErrManifestNoMetadata
	}
	namespace, ok = metadata["namespace"].(string)
	if !ok {
		namespace = ""
	}
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), namespace)
	if err != nil {
		return nil, err
	}
	name := metadata["name"].(string)
	foundObj, err := resourceInterface.Get(ctx, name, v1.GetOptions{
		TypeMeta: v1.TypeMeta{
			Kind:       obj.GetKind(),
			APIVersion: obj.GetAPIVersion(),
		},
	})
	if err != nil {
		return nil, err
	}
	return foundObj, nil
}

func (client *DynamicClient) RESTMapper() meta.RESTMapper {
	return client.restMapper
}

func (client *DynamicClient) resourceInterface(
	gvk schema.GroupVersionKind,
	namespace string,
) (dynamic.ResourceInterface, error) {
	restMapper := client.restMapper
	dynamicClient := client.dynamicClient
	mapping, err := restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		return dynamicClient.Resource(mapping.Resource).Namespace(namespace), nil
	}
	return dynamicClient.Resource(mapping.Resource), nil
}

// HelmClient is a dedicated Kubernetes client for Helm with Server-Side Apply.
// TODO: remove when Helm supports SSA.
type HelmClient struct {
	*helmKube.Client
	DynamicClient Client[unstructured.Unstructured]
	FieldManager  string
}

var _ helmKube.Interface = (*HelmClient)(nil)

var ErrObjectNotUnstructured = errors.New("Helm object is not of type unstructured.Unstructured")

// taken from helm.sh/helm/v3/pkg/kube and patched with SSA.
func (c *HelmClient) Create(resources helmKube.ResourceList) (*helmKube.Result, error) {
	ctx := context.Background()
	for _, info := range resources {
		if _, ok := info.Object.(*unstructured.Unstructured); !ok {
			return nil, ErrObjectNotUnstructured
		}
		if err := c.DynamicClient.Apply(ctx, info.Object.(*unstructured.Unstructured), c.FieldManager); err != nil {
			return nil, err
		}
	}
	return &helmKube.Result{Created: resources}, nil
}

var metadataAccessor = meta.NewAccessor()

// taken from helm.sh/helm/v3/pkg/kube and patched with SSA.
func (c *HelmClient) Update(
	original helmKube.ResourceList,
	target helmKube.ResourceList,
	force bool,
) (*helmKube.Result, error) {
	ctx := context.Background()
	res := &helmKube.Result{}
	err := target.Visit(func(info *resource.Info, err error) error {
		if _, ok := info.Object.(*unstructured.Unstructured); !ok {
			return ErrObjectNotUnstructured
		}
		// Append the created resource to the results, even if something fails
		res.Created = append(res.Created, info)
		if err := c.DynamicClient.Apply(ctx, info.Object.(*unstructured.Unstructured), c.FieldManager, Force(true)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	for _, info := range original.Difference(target) {
		c.Log(
			"Deleting %s %q in namespace %s...",
			info.Mapping.GroupVersionKind.Kind,
			info.Name,
			info.Namespace,
		)

		if err := info.Get(); err != nil {
			c.Log("Unable to get obj %q, err: %s", info.Name, err)
			continue
		}
		annotations, err := metadataAccessor.Annotations(info.Object)
		if err != nil {
			c.Log("Unable to get annotations on %q, err: %s", info.Name, err)
		}
		if annotations != nil && annotations[helmKube.ResourcePolicyAnno] == helmKube.KeepPolicy {
			c.Log(
				"Skipping delete of %q due to annotation [%s=%s]",
				info.Name,
				helmKube.ResourcePolicyAnno,
				helmKube.KeepPolicy,
			)
			continue
		}
		if err := c.deleteResource(info, v1.DeletePropagationBackground); err != nil {
			c.Log("Failed to delete %q, err: %s", info.ObjectName(), err)
			continue
		}
		res.Deleted = append(res.Deleted, info)
	}
	return res, nil
}

func (c *HelmClient) deleteResource(info *resource.Info, policy v1.DeletionPropagation) error {
	opts := &v1.DeleteOptions{PropagationPolicy: &policy}
	_, err := resource.NewHelper(info.Client, info.Mapping).
		WithFieldManager(c.FieldManager).
		DeleteWithOptions(info.Namespace, info.Name, opts)
	return err
}
