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

package component_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kharf/declcd/pkg/component"
	"gotest.tools/v3/assert"
)

func TestDependencyGraph_Insert(t *testing.T) {
	testCases := []struct {
		name        string
		nodes       []component.Component
		expectedErr error
	}{
		{
			name: "NoConflict",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{},
					Content:      int32(0),
				},
				component.Component{
					ID:           "linkerd___Namespace",
					Dependencies: []string{"certmanager"},
					Content:      int32(1),
				},
			},
			expectedErr: nil,
		},
		{
			name: "Conflict",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{},
					Content:      int32(0),
				},
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{"certmanager"},
					Content:      int32(1),
				},
				component.Component{
					ID:           "shouldntmatter___Namespace",
					Dependencies: []string{},
					Content:      int32(2),
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
		node               *component.Component
	}{
		{
			name:               "Found",
			componentIDRequest: "prometheus___Namespace",
			node: &component.Component{
				ID:      "prometheus___Namespace",
				Content: int32(0),
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
				err := graph.Insert(*tc.node)
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
		component.Component{
			ID:           "prometheus___Namespace",
			Dependencies: []string{},
			Content:      int32(0),
		},
		component.Component{
			ID:           "linkerd___Namespace",
			Dependencies: []string{"certmanager"},
			Content:      int32(1),
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
		nodes []component.Component
		err   error
	}{
		{
			name: "Positive",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{},
					Content:      int32(0),
				},
				component.Component{
					ID:           "linkerd___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(1),
				},
				component.Component{
					ID:           "certmanager___Namespace",
					Dependencies: []string{},
					Content:      int32(2),
				},
				component.Component{
					ID:           "emissaryingress___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(3),
				},
				component.Component{
					ID:           "keda___Namespace",
					Dependencies: []string{"prometheus___Namespace"},
					Content:      int32(4),
				},
			},
			err: nil,
		}, {
			name: "UnknownDependencyID",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{},
					Content:      int32(0),
				},
				component.Component{
					ID:           "linkerd___Namespace",
					Dependencies: []string{"certmanager"},
					Content:      int32(1),
				},
				component.Component{
					ID:           "certmanager___Namespace",
					Dependencies: []string{},
					Content:      int32(2),
				},
			},
			err: component.ErrUnknownComponentID,
		},
		{
			name: "Cycle",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{},
					Content:      int32(0),
				},
				component.Component{
					ID:           "linkerd___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(1),
				},
				component.Component{
					ID:           "certmanager___Namespace",
					Dependencies: []string{"linkerd___Namespace"},
					Content:      int32(2),
				},
				component.Component{
					ID:           "emissaryingress___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(3),
				},
				component.Component{
					ID:           "keda___Namespace",
					Dependencies: []string{"prometheus___Namespace"},
					Content:      int32(4),
				},
			},
			err: component.ErrCyclicDependency,
		},
		{
			name: "DistantCycle",
			nodes: []component.Component{
				component.Component{
					ID:           "prometheus___Namespace",
					Dependencies: []string{"keda___Namespace"},
					Content:      int32(0),
				},
				component.Component{
					ID:           "linkerd___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(1),
				},
				component.Component{
					ID:           "certmanager___Namespace",
					Dependencies: []string{},
					Content:      int32(2),
				},
				component.Component{
					ID:           "emissaryingress___Namespace",
					Dependencies: []string{"certmanager___Namespace"},
					Content:      int32(3),
				},
				component.Component{
					ID:           "keda___Namespace",
					Dependencies: []string{"prometheus___Namespace"},
					Content:      int32(4),
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
					for _, dep := range n.Dependencies {
						_, found := visited[dep]
						assert.Assert(t, found)
					}
					visited[n.ID] = struct{}{}
				}
			}
		})
	}
}
