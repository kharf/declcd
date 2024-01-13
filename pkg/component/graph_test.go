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
	graph := component.NewDependencyGraph()
	err := graph.Insert(
		component.NewNode("prometheus", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("linkerd", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("certmanager", "", []string{}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("emissaryingress", "", []string{"certmanager"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
		component.NewNode("keda", "", []string{"prometheus"}, []component.ManifestMetadata{}, []component.HelmReleaseMetadata{}),
	)
	assert.NilError(t, err)
	result, err := graph.TopologicalSort()
	assert.NilError(t, err)
	assert.Assert(t, len(result) == 5)
	visited := make(map[string]struct{})
	for _, n := range result {
		for _, dep := range n.Dependencies() {
			_, found := visited[dep]
			assert.Assert(t, found)
		}
		visited[n.ID()] = struct{}{}
	}
}
