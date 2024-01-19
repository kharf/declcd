package component_test

import (
	"testing"

	"github.com/kharf/declcd/pkg/component"
	"gotest.tools/v3/assert"
)

func TestDependencyGraph_Insert(t *testing.T) {
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		component.NewNode("prometheus", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("prometheus", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
	)
	assert.ErrorIs(t, err, component.ErrDuplicateComponentID)
}

func TestDependencyGraph_Delete(t *testing.T) {
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		component.NewNode("prometheus", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
	)
	assert.NilError(t, err)
	graph.Delete("prometheus")
	node := graph.Get("prometheus")
	assert.Assert(t, node == nil)
}

func TestDependencyGraph_TopologicalSort(t *testing.T) {
	testCases := []struct {
		name  string
		nodes []component.Node
		err   error
	}{
		{
			name: "Positive",
			nodes: []component.Node{
				component.NewNode("prometheus", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
			},
			err: nil,
		},
		{
			name: "Cycle",
			nodes: []component.Node{
				component.NewNode("prometheus", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{"linkerd"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
			},
			err: component.ErrCyclicDependency,
		},
		{
			name: "DistantCycle",
			nodes: []component.Node{
				component.NewNode("prometheus", "", []string{"keda"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{"emissaryingress"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"keda"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
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
					for _, dep := range n.Dependencies() {
						_, found := visited[dep]
						assert.Assert(t, found)
					}
					visited[n.ID()] = struct{}{}
				}
			}
		})
	}
}
