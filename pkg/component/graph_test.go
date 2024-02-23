package component_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kharf/declcd/pkg/component"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDependencyGraph_Insert(t *testing.T) {
	testCases := []struct {
		name        string
		nodes       []component.Instance
		expectedErr error
	}{
		{
			name: "NoConflict",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "prometheus",
							},
						},
					},
				},
				&component.Manifest{
					"linkerd___Namespace",
					[]string{"certmanager"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			name: "Conflict",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "prometheus",
							},
						},
					},
				},
				&component.Manifest{
					"prometheus___Namespace",
					[]string{"certmanager"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "prometheus",
							},
						},
					},
				},
				&component.Manifest{
					"shouldntmatter___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "prometheus",
							},
						},
					},
				},
			},
			expectedErr: component.ErrDuplicateComponentID,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			graph := component.NewDependencyGraph()
			err := graph.Insert(tc.nodes...)
			if tc.expectedErr == nil {
				assert.NilError(t, err)
			} else {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}

func TestDependencyGraph_Get(t *testing.T) {
	testCases := []struct {
		name               string
		componentIDRequest string
		node               component.Instance
	}{
		{
			name:               "Found",
			componentIDRequest: "prometheus___Namespace",
			node: &component.Manifest{
				ID: "prometheus___Namespace",
				Content: unstructured.Unstructured{
					Object: map[string]interface{}{
						"kind":       "Namespace",
						"apiVersion": "v1",
						"metadata": map[string]interface{}{
							"name": "prometheus",
						},
					},
				},
			},
		},
		{
			name:               "NotFound",
			componentIDRequest: "prometheus___v1_Namespace",
			node:               nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			graph := component.NewDependencyGraph()
			if tc.node != nil {
				err := graph.Insert(tc.node)
				assert.NilError(t, err)
			}
			node := graph.Get(tc.componentIDRequest)
			assert.Assert(t, cmp.Equal(node, tc.node))
		})
	}
}

func TestDependencyGraph_Delete(t *testing.T) {
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		&component.Manifest{
			"prometheus___Namespace",
			[]string{},
			unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Namespace",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name": "prometheus",
					},
				},
			},
		},
		&component.Manifest{
			"linkerd___Namespace",
			[]string{"certmanager"},
			unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Namespace",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name": "linkerd",
					},
				},
			},
		},
	)
	assert.NilError(t, err)
	graph.Delete("prometheus___Namespace")
	node := graph.Get("prometheus___Namespace")
	assert.Assert(t, node == nil)
	node = graph.Get("linkerd___Namespace")
	assert.Assert(t, node != nil)
}

func TestDependencyGraph_TopologicalSort(t *testing.T) {
	testCases := []struct {
		name  string
		nodes []component.Instance
		err   error
	}{
		{
			name: "Positive",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"linkerd___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"certmanager___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "certmanager",
							},
						},
					},
				},
				&component.Manifest{
					"emissaryingress___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "emissaryingress",
							},
						},
					},
				},
				&component.Manifest{
					"keda___Namespace",
					[]string{"prometheus___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "keda",
							},
						},
					},
				},
			},
			err: nil,
		}, {
			name: "UnknownDependencyID",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"linkerd___Namespace",
					[]string{"certmanager"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"certmanager___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "certmanager",
							},
						},
					},
				},
			},
			err: component.ErrUnknownComponentID,
		},
		{
			name: "Cycle",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"linkerd___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"certmanager___Namespace",
					[]string{"linkerd___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "certmanager",
							},
						},
					},
				},
				&component.Manifest{
					"emissaryingress___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "emissaryingress",
							},
						},
					},
				},
				&component.Manifest{
					"keda___Namespace",
					[]string{"prometheus___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "keda",
							},
						},
					},
				},
			},
			err: component.ErrCyclicDependency,
		},
		{
			name: "DistantCycle",
			nodes: []component.Instance{
				&component.Manifest{
					"prometheus___Namespace",
					[]string{"keda___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"linkerd___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "linkerd",
							},
						},
					},
				},
				&component.Manifest{
					"certmanager___Namespace",
					[]string{},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "certmanager",
							},
						},
					},
				},
				&component.Manifest{
					"emissaryingress___Namespace",
					[]string{"certmanager___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "emissaryingress",
							},
						},
					},
				},
				&component.Manifest{
					"keda___Namespace",
					[]string{"prometheus___Namespace"},
					unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": "keda",
							},
						},
					},
				},
			},
			err: component.ErrCyclicDependency,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			graph := component.NewDependencyGraph()
			err := graph.Insert(
				tc.nodes...,
			)
			assert.NilError(t, err)
			result, err := graph.TopologicalSort()
			if tc.err != nil {
				assert.ErrorIs(t, err, tc.err)
			} else {
				assert.Assert(t, len(result) == len(tc.nodes))
				visited := make(map[string]struct{})
				for _, n := range result {
					for _, dep := range n.GetDependencies() {
						_, found := visited[dep]
						assert.Assert(t, found)
					}
					visited[n.GetID()] = struct{}{}
				}
			}
		})
	}
}
