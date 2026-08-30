// Package dag provides dependency validation and readiness for contract task
// IDs without depending on the contract package.
package dag

import "strings"

// Node is the contract-independent portion of a task needed by the graph.
type Node struct {
	ID        string
	DependsOn []string
}

// Violation identifies a dependency graph problem.
type Violation struct {
	Code       string
	NodeID     string
	Dependency string
}

const (
	MissingDependency = "missing_dependency"
	DependencyCycle   = "dependency_cycle"
)

// Graph retains nodes in their original contract order.
type Graph struct {
	nodes []Node
}

// Build validates dependency references and cycles, returning the graph even
// when violations are present so callers can inspect deterministic readiness.
func Build(nodes []Node) (Graph, []Violation) {
	graph := Graph{nodes: cloneNodes(nodes)}
	byID := make(map[string]struct{}, len(nodes))
	var violations []Violation
	for _, node := range nodes {
		if _, exists := byID[node.ID]; exists {
			// Task ID uniqueness is validated by the contract package. Keep the
			// graph independent and let that layer own its public diagnostic.
			continue
		}
		byID[node.ID] = struct{}{}
	}

	for _, node := range nodes {
		for _, dependency := range node.DependsOn {
			if strings.TrimSpace(dependency) == "" {
				violations = append(violations, Violation{Code: MissingDependency, NodeID: node.ID, Dependency: dependency})
				continue
			}
			if _, exists := byID[dependency]; !exists {
				violations = append(violations, Violation{Code: MissingDependency, NodeID: node.ID, Dependency: dependency})
			}
		}
	}

	if hasCycle(nodes, byID) {
		violations = append(violations, Violation{Code: DependencyCycle})
	}
	return graph, violations
}

// Ready returns incomplete nodes whose dependencies are all complete, in the
// same order as the input to Build.
func (g Graph) Ready(completed map[string]bool) []string {
	ready := make([]string, 0, len(g.nodes))
	for _, node := range g.nodes {
		if completed[node.ID] {
			continue
		}
		satisfied := true
		for _, dependency := range node.DependsOn {
			if !completed[dependency] {
				satisfied = false
				break
			}
		}
		if satisfied {
			ready = append(ready, node.ID)
		}
	}
	return ready
}

func cloneNodes(nodes []Node) []Node {
	cloned := make([]Node, len(nodes))
	for i, node := range nodes {
		cloned[i] = node
		cloned[i].DependsOn = append([]string(nil), node.DependsOn...)
	}
	return cloned
}

// hasCycle uses Kahn's algorithm. Missing dependencies are not edges and are
// already reported separately above.
func hasCycle(nodes []Node, byID map[string]struct{}) bool {
	indegree := make(map[string]int, len(byID))
	adjacency := make(map[string][]string, len(byID))
	for id := range byID {
		indegree[id] = 0
	}
	for _, node := range nodes {
		if _, exists := byID[node.ID]; !exists {
			continue
		}
		for _, dependency := range node.DependsOn {
			if _, exists := byID[dependency]; !exists {
				continue
			}
			adjacency[dependency] = append(adjacency[dependency], node.ID)
			indegree[node.ID]++
		}
	}

	queue := make([]string, 0, len(indegree))
	for _, node := range nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, child := range adjacency[id] {
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	return processed < len(byID)
}
