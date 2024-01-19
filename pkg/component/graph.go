package component

import (
	"errors"
	"fmt"
)

var (
	ErrCyclicDependency     = errors.New("Cyclic dependency detected")
	ErrDuplicateComponentID = errors.New("Duplicate Component ID")
)

// DependencyGraph is an adjacency list which represents the directed acyclic graph of component dependencies.
// The Dependencies field in the Node struct holds a list of other component ids to which the current component has edges.
type DependencyGraph struct {
	set map[string]Node
}

func NewDependencyGraph() DependencyGraph {
	return DependencyGraph{
		set: make(map[string]Node),
	}
}

func (graph DependencyGraph) Insert(nodes ...Node) error {
	for _, node := range nodes {
		if _, found := graph.set[node.id]; found {
			return fmt.Errorf("%w: %s already exists in set", ErrDuplicateComponentID, node.id)
		}
		graph.set[node.id] = node
	}
	return nil
}

func (graph DependencyGraph) Delete(componentID string) {
	delete(graph.set, componentID)
}

func (graph DependencyGraph) Get(componentID string) *Node {
	node, found := graph.set[componentID]
	if !found {
		return nil
	}
	return &node
}

// TopologicalSort performs a topological sort on the component dependency graph and returns the sorted order.
// It returns an error if a cycle is detected.
func (dag DependencyGraph) TopologicalSort() ([]Node, error) {
	inProcessing := make(map[string]struct{})
	visited := make(map[string]struct{}, len(dag.set))
	result := make([]Node, 0, len(dag.set))
	var walk func(nodeID string) error
	walk = func(nodeID string) error {
		if _, found := inProcessing[nodeID]; found {
			return fmt.Errorf("%w for %s", ErrCyclicDependency, nodeID)
		}
		if _, found := visited[nodeID]; found {
			return nil
		}
		inProcessing[nodeID] = struct{}{}
		node := dag.set[nodeID]
		for _, depNode := range node.dependencies {
			if err := walk(depNode); err != nil {
				return err
			}
		}
		delete(inProcessing, nodeID)
		visited[nodeID] = struct{}{}
		result = append(result, node)
		return nil
	}
	for node := range dag.set {
		if err := walk(node); err != nil {
			return nil, err
		}
	}
	return result, nil
}
