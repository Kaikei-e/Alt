// Package render serializes the supersedes DAG. Mermaid output is
// byte-compatible with scripts/adr_graph.py's render_mermaid so the
// Python->Go cutover never silently changes rendered docs.
package render

import (
	"github.com/alt-project/adrdag/internal/adr"
)

// Mermaid renders the DAG as a fenced mermaid block (no trailing newline).
func Mermaid(graph map[string][]string, adrs map[string]adr.ADR) string {
	return "" // stub: RED
}

// DOT renders the DAG as Graphviz digraph source (no trailing newline).
func DOT(graph map[string][]string) string {
	return "" // stub: RED
}

// JSONGraph renders nodes + links in NetworkX-style node-link form.
// The edge array key is "links", not "edges" — that is the actual
// NetworkX node-link default (see networkx/networkx#8611).
func JSONGraph(adrs map[string]adr.ADR, graph, reverse map[string][]string) ([]byte, error) {
	return nil, nil // stub: RED
}
