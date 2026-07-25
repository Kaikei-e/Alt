// Package render serializes the supersedes DAG. Mermaid output is
// byte-compatible with scripts/adr_graph.py's render_mermaid so the
// Python->Go cutover never silently changes rendered docs.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
)

// mermaidLabel mirrors adr_graph.py's _mermaid_label: "id: title" with the
// title truncated to 37 runes + "..." when longer than 40 runes (Python
// len() counts characters, not bytes) and double quotes replaced by singles.
func mermaidLabel(id string, adrs map[string]adr.ADR) string {
	title := adrs[id].Title
	if title == "" {
		return id
	}
	runes := []rune(title)
	if len(runes) > 40 {
		title = string(runes[:37]) + "..."
	}
	title = strings.ReplaceAll(title, `"`, `'`)
	return fmt.Sprintf("%s: %s", id, title)
}

func sortedNewIDs(g map[string][]string) []string {
	ids := make([]string, 0, len(g))
	for id := range g {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Mermaid renders the DAG as a fenced mermaid block (no trailing newline),
// byte-identical to adr_graph.py's render_mermaid.
func Mermaid(g map[string][]string, adrs map[string]adr.ADR) string {
	lines := []string{"```mermaid", "graph LR"}
	for _, newID := range sortedNewIDs(g) {
		targets := append([]string{}, g[newID]...)
		sort.Strings(targets)
		for _, oldID := range targets {
			lines = append(lines, fmt.Sprintf(
				`    %s["%s"] -->|superseded by| %s["%s"]`,
				oldID, mermaidLabel(oldID, adrs), newID, mermaidLabel(newID, adrs)))
		}
	}
	lines = append(lines, "```")
	return strings.Join(lines, "\n")
}

// DOT renders the DAG as Graphviz digraph source (no trailing newline),
// edges in the same old -->"superseded by"--> new direction as mermaid.
func DOT(g map[string][]string) string {
	lines := []string{"digraph adr_supersedes {", "    rankdir=LR;"}
	for _, newID := range sortedNewIDs(g) {
		targets := append([]string{}, g[newID]...)
		sort.Strings(targets)
		for _, oldID := range targets {
			lines = append(lines, fmt.Sprintf(`    "%s" -> "%s" [label="superseded by"];`, oldID, newID))
		}
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

type jsonNode struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Binding bool   `json:"binding"`
}

type jsonLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type jsonGraph struct {
	Nodes []jsonNode `json:"nodes"`
	Links []jsonLink `json:"links"`
}

// JSONGraph renders nodes + links in NetworkX-style node-link form.
// The edge array key is "links", not "edges" — that is the actual
// NetworkX node-link default (see networkx/networkx#8611). Link direction
// is the authored one: source = the new ADR, target = the ADR it supersedes.
func JSONGraph(adrs map[string]adr.ADR, g, reverse map[string][]string) ([]byte, error) {
	doc := jsonGraph{Nodes: []jsonNode{}, Links: []jsonLink{}}
	for _, id := range adr.SortedIDs(adrs) {
		a := adrs[id]
		doc.Nodes = append(doc.Nodes, jsonNode{
			ID:      id,
			Title:   a.Title,
			Status:  a.Status,
			Binding: graph.IsBinding(a.Status, id, reverse),
		})
	}
	for _, newID := range sortedNewIDs(g) {
		targets := append([]string{}, g[newID]...)
		sort.Strings(targets)
		for _, oldID := range targets {
			doc.Links = append(doc.Links, jsonLink{Source: newID, Target: oldID, Type: "supersedes"})
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}
