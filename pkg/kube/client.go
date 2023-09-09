package kube

import (
	"context"

	"helm.sh/helm/v3/pkg/action"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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

const (
	ClientName = "declcd-controller"
)

// Client connects to a Kubernetes cluster to create, read, update and delete unstructured manifests/objects.
type Client struct {
	dynamicClient *dynamic.DynamicClient
	client        *kubernetes.Clientset
	RestMapper    meta.RESTMapper
}

func NewClient(config *rest.Config) (*Client, error) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return &Client{
		dynamicClient: dynClient,
		client:        client,
		RestMapper:    restMapper,
	}, nil
}

// Apply applies changes to an object through a Server-Side Apply and takes the ownership of this object.
func (client *Client) Apply(ctx context.Context, obj *unstructured.Unstructured) error {
	restMapper := client.RestMapper
	dynamicClient := client.dynamicClient

	gvk := obj.GroupVersionKind()
	mapping, err := restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var resourceInterface dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		resourceInterface = dynamicClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	} else {
		resourceInterface = dynamicClient.Resource(mapping.Resource)
	}

	_, err = resourceInterface.Create(ctx, obj, v1.CreateOptions{FieldManager: ClientName})
	if err != nil {
		if errors.ReasonForError(err) == v1.StatusReasonAlreadyExists {
			_, err = resourceInterface.Update(ctx, obj, v1.UpdateOptions{FieldManager: ClientName})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

// WIP
func (client *Client) List(ctx context.Context, opts v1.ListOptions) ([]appsv1.Deployment, error) {
	deployments := client.client.AppsV1().Deployments("default")

	list, err := deployments.List(ctx, v1.ListOptions{})
	if err != nil {
		return []appsv1.Deployment{}, err
	}

	return list.Items, nil
}
