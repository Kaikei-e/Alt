// Package adr loads ADR markdown files and their flat YAML-ish frontmatter.
//
// The parser is deliberately line-based — a direct port of
// scripts/adr_graph.py's parse_frontmatter — NOT a strict YAML decoder:
// 18 of the 950 real ADRs carry unquoted scalars (backticks, ": " inside
// prose) that strict YAML rejects but the canonical Python tool accepts.
package adr

// ADR is one loaded decision record.
type ADR struct {
	ID                  string
	Title               string
	Status              string
	Supersedes          []string
	EmptySupersedesStub bool
}

// Frontmatter is the parsed flat frontmatter block.
type Frontmatter struct {
	Fields        map[string]string
	Lists         map[string][]string
	EmptyListKeys map[string]bool
}

// ParseFrontmatter parses the leading frontmatter block of an ADR file.
func ParseFrontmatter(content string) Frontmatter {
	return Frontmatter{} // stub: RED
}

// NormalizeID normalizes any ADR id spelling (`339`, `ADR-339`) to `000339`.
func NormalizeID(raw string) string {
	return "" // stub: RED
}

// LoadDir loads every NNNNNN.md file under dir into id -> ADR.
func LoadDir(dir string) (map[string]ADR, error) {
	return nil, nil // stub: RED
}

// SupersedesGraph builds new-id -> [old-id...] adjacency from loaded ADRs.
func SupersedesGraph(adrs map[string]ADR) map[string][]string {
	return nil // stub: RED
}

// SortedIDs returns all ADR ids in ascending order.
func SortedIDs(adrs map[string]ADR) []string {
	return nil // stub: RED
}
