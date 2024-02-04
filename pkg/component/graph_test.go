package component_test

import (
	"testing"

	"github.com/kharf/declcd/pkg/component"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"gotest.tools/v3/assert"
)

func TestDependencyGraph_Insert(t *testing.T) {
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		component.NewNode("prometheus", "", []string{}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
		component.NewNode("linkerd", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
		component.NewNode("prometheus", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
	)
	assert.ErrorIs(t, err, component.ErrDuplicateComponentID)
}

func TestDependencyGraph_Delete(t *testing.T) {
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		component.NewNode("prometheus", "", []string{}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
		component.NewNode("linkerd", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
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
				component.NewNode("prometheus", "", []string{}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
			},
			err: nil,
		},
		{
			name: "Cycle",
			nodes: []component.Node{
				component.NewNode("prometheus", "", []string{}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{"linkerd"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
			},
			err: component.ErrCyclicDependency,
		},
		{
			name: "DistantCycle",
			nodes: []component.Node{
				component.NewNode("prometheus", "", []string{"keda"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("linkerd", "", []string{"certmanager"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("certmanager", "", []string{"emissaryingress"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("emissaryingress", "", []string{"keda"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
				component.NewNode("keda", "", []string{"prometheus"}, []kube.ManifestMetadata{}, []helm.ReleaseMetadata{}),
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
