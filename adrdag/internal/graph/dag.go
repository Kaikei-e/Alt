// Package graph implements the supersedes DAG algorithms, semantics-compatible
// with scripts/adr_graph.py: reverse edges are always computed, never authored,
// and binding(A) ⇔ status=accepted ∧ no inbound supersedes edge.
package graph

import "sort"

// sortedKeys returns map keys in ascending order. adr_graph.py gets
// deterministic iteration for free from sorted dict insertion; Go maps
// randomize, so every walk here iterates sorted keys explicitly.
func sortedKeys(g map[string][]string) []string {
	keys := make([]string, 0, len(g))
	for k := range g {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// BuildReverse inverts new->old adjacency into old -> [new ids superseding it].
// Every node of g gets an entry; edge targets absent from g are added.
func BuildReverse(g map[string][]string) map[string][]string {
	reverse := make(map[string][]string, len(g))
	for node := range g {
		reverse[node] = []string{}
	}
	for _, newID := range sortedKeys(g) {
		for _, oldID := range g[newID] {
			reverse[oldID] = append(reverse[oldID], newID)
		}
	}
	return reverse
}

// FindCycle returns a closed cycle path (e.g. [A B A]) or nil when acyclic.
// Three-color DFS, a direct port of adr_graph.py's find_cycle.
func FindCycle(g map[string][]string) []string {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(g))
	var pathStack []string

	var visit func(node string) []string
	visit = func(node string) []string {
		color[node] = gray
		pathStack = append(pathStack, node)
		for _, neighbor := range g[node] {
			switch color[neighbor] {
			case white:
				if found := visit(neighbor); found != nil {
					return found
				}
			case gray:
				start := 0
				for i, n := range pathStack {
					if n == neighbor {
						start = i
						break
					}
				}
				cycle := append([]string{}, pathStack[start:]...)
				return append(cycle, neighbor)
			}
		}
		pathStack = pathStack[:len(pathStack)-1]
		color[node] = black
		return nil
	}

	for _, node := range sortedKeys(g) {
		if color[node] == white {
			if found := visit(node); found != nil {
				return found
			}
		}
	}
	return nil
}

// Resolve walks the reverse graph forward to the currently-effective terminal
// ADRs. A terminal (no successors) resolves to itself; fan-in convergence is
// de-duplicated preserving first-seen order, like adr_graph.py's resolve().
// Unlike the Python original it is safe on cyclic input: successors already on
// the current walk path are skipped instead of recursing forever.
func Resolve(id string, reverse map[string][]string) []string {
	return resolve(id, reverse, map[string]bool{})
}

func resolve(id string, reverse map[string][]string, onPath map[string]bool) []string {
	successors := reverse[id]
	if len(successors) == 0 {
		return []string{id}
	}
	onPath[id] = true
	defer delete(onPath, id)
	terminal := []string{}
	seen := map[string]bool{}
	for _, successor := range successors {
		if onPath[successor] {
			continue
		}
		for _, leaf := range resolve(successor, reverse, onPath) {
			if !seen[leaf] {
				seen[leaf] = true
				terminal = append(terminal, leaf)
			}
		}
	}
	return terminal
}

// IsBinding reports whether an ADR is a currently-binding decision:
// status=accepted AND no inbound supersedes edge.
func IsBinding(status, id string, reverse map[string][]string) bool {
	return status == "accepted" && len(reverse[id]) == 0
}
