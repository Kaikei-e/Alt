// Package graph implements the supersedes DAG algorithms, semantics-compatible
// with scripts/adr_graph.py: reverse edges are always computed, never authored,
// and binding(A) ⇔ status=accepted ∧ no inbound supersedes edge.
package graph

// BuildReverse inverts new->old adjacency into old -> [new ids superseding it].
func BuildReverse(g map[string][]string) map[string][]string {
	return nil // stub: RED
}

// FindCycle returns a closed cycle path (e.g. [A B A]) or nil when acyclic.
func FindCycle(g map[string][]string) []string {
	return nil // stub: RED
}

// Resolve walks the reverse graph to the currently-effective terminal ADRs.
func Resolve(id string, reverse map[string][]string) []string {
	return nil // stub: RED
}

// IsBinding reports whether an ADR is a currently-binding decision.
func IsBinding(status, id string, reverse map[string][]string) bool {
	return false // stub: RED
}
