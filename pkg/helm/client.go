// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helm

import (
	"context"
	"errors"

	"github.com/kharf/declcd/pkg/kube"
	helmKube "helm.sh/helm/v3/pkg/kube"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
)

// Client is a dedicated Kubernetes client for Helm with Server-Side Apply.
// TODO: remove when Helm supports SSA.
type Client struct {
	*helmKube.Client
	DynamicClient kube.Client[kube.ExtendedUnstructured, unstructured.Unstructured]
	FieldManager  string
	Patches       *Patches
}

var _ helmKube.Interface = (*Client)(nil)

var ErrObjectNotUnstructured = errors.New("Helm object is not of type unstructured.Unstructured")

// taken from helm.sh/helm/v3/pkg/kube and patched with SSA.
func (c *Client) Create(resources helmKube.ResourceList) (*helmKube.Result, error) {
	ctx := context.Background()
	for _, info := range resources {
		unstr, ok := info.Object.(*unstructured.Unstructured)
		if !ok {
			return nil, ErrObjectNotUnstructured
		}

		if err := c.apply(ctx, unstr); err != nil {
			return nil, err
		}
	}
	return &helmKube.Result{Created: resources}, nil
}

func (c *Client) apply(ctx context.Context, unstr *unstructured.Unstructured) error {
	var patch *kube.ExtendedUnstructured
	if c.Patches != nil {
		patch = c.Patches.Get(unstr.GetName(), unstr.GetNamespace(), v1.TypeMeta{
			APIVersion: unstr.GetAPIVersion(),
			Kind:       unstr.GetKind(),
		})
	}

	extendedUnstr := &kube.ExtendedUnstructured{}
	if patch != nil {
		extendedUnstr.Metadata = patch.Metadata
	}
	extendedUnstr.Unstructured = unstr

	if err := c.DynamicClient.Apply(ctx, extendedUnstr, c.FieldManager, kube.Force(true)); err != nil {
		return err
	}

	return nil
}

var metadataAccessor = meta.NewAccessor()

// taken from helm.sh/helm/v3/pkg/kube and patched with SSA.
func (c *Client) Update(
	original helmKube.ResourceList,
	target helmKube.ResourceList,
	force bool,
) (*helmKube.Result, error) {
	ctx := context.Background()
	res := &helmKube.Result{}
	err := target.Visit(func(info *resource.Info, err error) error {
		unstr, ok := info.Object.(*unstructured.Unstructured)
		if !ok {
			return ErrObjectNotUnstructured
		}

		// Append the created resource to the results, even if something fails
		res.Created = append(res.Created, info)
		if err := c.apply(ctx, unstr); err != nil {
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

func (c *Client) deleteResource(info *resource.Info, policy v1.DeletionPropagation) error {
	opts := &v1.DeleteOptions{PropagationPolicy: &policy}
	_, err := resource.NewHelper(info.Client, info.Mapping).
		WithFieldManager(c.FieldManager).
		DeleteWithOptions(info.Namespace, info.Name, opts)
	return err
}
