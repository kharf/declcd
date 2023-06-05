package kube

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

const (
	ClientName = "declcd-controller"
)

// Client connects to a Kubernetes cluster to create, read, update and delete unstructured manifests/objects.
type Client struct {
	dynamicClient *dynamic.DynamicClient
	restMapper    meta.RESTMapper
}

func NewClient(config *rest.Config) (*Client, error) {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return &Client{
		dynamicClient: client,
		restMapper:    restMapper,
	}, nil
}

// Apply applies changes to an object through a Server-Side Apply and takes the ownership of this object.
func (client *Client) Apply(ctx context.Context, obj *unstructured.Unstructured) error {
	restMapper := client.restMapper
	dynamicClient := client.dynamicClient
	fmt.Println("object: ", obj.GetKind())

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

	list, err := resourceInterface.List(ctx, v1.ListOptions{})
	if err != nil {
		return err
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

	for _, unstr := range list.Items {
		fmt.Println("found: ", unstr.GetName())
	}

	return nil
}
