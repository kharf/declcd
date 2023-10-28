package kube

import (
	"context"
	"errors"
	"slices"
	"time"

	"helm.sh/helm/v3/pkg/action"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
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

func (c InMemoryRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.Cfg, nil
}

func (c InMemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	client, err := discovery.NewDiscoveryClientForConfig(c.Cfg)
	return memory.NewMemCacheClient(client), err
}

func (c InMemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return c.RestMapper, nil
}

func (c InMemoryRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}

// Client connects to a Kubernetes cluster to create, read, update and delete manifests/objects.
type Client[T any] interface {
	Apply(ctx context.Context, obj *T, fieldManager string) error
	Get(ctx context.Context, obj *T) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, obj *T) error
}

// DynamicClient connects to a Kubernetes cluster to create, read, update and delete unstructured manifests/objects.
type DynamicClient struct {
	dynamicClient *dynamic.DynamicClient
	RestMapper    meta.RESTMapper
	invalidate    func()
}

var _ Client[unstructured.Unstructured] = (*DynamicClient)(nil)

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
		RestMapper:    restMapper,
		invalidate:    restMapper.Reset,
	}, nil
}

func (client *DynamicClient) Invalidate() error {
	client.invalidate()
	return nil
}

var ErrWaitingForResource = errors.New("error waiting for resource")

// Apply applies changes to an object through a Server-Side Apply and takes the ownership of this object.
func (client *DynamicClient) Apply(ctx context.Context, obj *unstructured.Unstructured, fieldManager string) error {
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return err
	}

	_, err = resourceInterface.Create(ctx, obj, v1.CreateOptions{FieldManager: fieldManager})
	if err != nil {
		if k8sErrors.ReasonForError(err) == v1.StatusReasonAlreadyExists {
			existingObj, err := resourceInterface.Get(ctx, obj.GetName(), v1.GetOptions{TypeMeta: v1.TypeMeta{
				Kind:       obj.GetKind(),
				APIVersion: obj.GetAPIVersion(),
			},
			})
			if err != nil {
				return err
			}
			obj.SetResourceVersion(existingObj.GetResourceVersion())
			_, err = resourceInterface.Update(ctx, obj, v1.UpdateOptions{FieldManager: fieldManager})
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

func (client *DynamicClient) wait(ctx context.Context, name string, typeMeta v1.TypeMeta, resourceInterface dynamic.ResourceInterface) (bool, error) {
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
		if cond.cType == string(apiextensionsv1.Established) {
			return true
		}
		return false
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

func (client *DynamicClient) Get(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	resourceInterface, err := client.resourceInterface(obj.GroupVersionKind(), obj.GetNamespace())
	if err != nil {
		return nil, err
	}

	foundObj, err := resourceInterface.Get(ctx, obj.GetName(), v1.GetOptions{
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

func (client *DynamicClient) resourceInterface(gvk schema.GroupVersionKind, namespace string) (dynamic.ResourceInterface, error) {
	restMapper := client.RestMapper
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
