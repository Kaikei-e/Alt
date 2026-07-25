// Package adr loads ADR markdown files and their flat YAML-ish frontmatter.
//
// The parser is deliberately line-based — a direct port of
// scripts/adr_graph.py's parse_frontmatter — NOT a strict YAML decoder:
// 18 of the 950 real ADRs carry unquoted scalars (backticks, ": " inside
// prose) that strict YAML rejects but the canonical Python tool accepts.
package adr

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

var (
	adrStemRE   = regexp.MustCompile(`^\d{6}$`)
	fieldRE     = regexp.MustCompile(`^([A-Za-z_]+):\s*(.*)$`)
	blockItemRE = regexp.MustCompile(`^\s+-\s*(.*)$`)
	nonDigitRE  = regexp.MustCompile(`\D`)
)

func stripQuotes(s string) string {
	s = strings.Trim(s, `"`)
	return strings.Trim(s, `'`)
}

// ParseFrontmatter parses the leading frontmatter block of an ADR file.
// Supports scalar strings, inline flow lists (`key: [a, b]`), and block
// lists (`key:` + indented `- item` lines) — the only shapes Alt's ADR
// frontmatter uses (mirrors adr_graph.py's parse_frontmatter).
func ParseFrontmatter(content string) Frontmatter {
	fm := Frontmatter{
		Fields:        map[string]string{},
		Lists:         map[string][]string{},
		EmptyListKeys: map[string]bool{},
	}
	// python reads files via Path.read_text(), which performs
	// universal-newline translation — mirror it so CRLF ADRs parse
	// identically instead of silently dropping their frontmatter
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return fm
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return fm
	}
	lines := strings.Split(content[4:4+end], "\n")
	for i := 0; i < len(lines); {
		m := fieldRE.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}
		key, value := m[1], strings.TrimSpace(m[2])
		// python keeps every field in one dict, so a repeated key keeps the
		// textually-last occurrence regardless of scalar/list shape — a later
		// write must evict the same key from the other map. _empty_list_keys
		// is append-only in python, so EmptyListKeys stays sticky on purpose.
		switch {
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			items := []string{}
			for _, part := range strings.Split(value[1:len(value)-1], ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					items = append(items, stripQuotes(part))
				}
			}
			delete(fm.Fields, key)
			fm.Lists[key] = items
			i++
		case value == "":
			items := []string{}
			sawEmptyItem := false
			j := i + 1
			for j < len(lines) {
				im := blockItemRE.FindStringSubmatch(lines[j])
				if im == nil {
					break
				}
				item := stripQuotes(strings.TrimSpace(im[1]))
				if item != "" {
					items = append(items, item)
				} else {
					sawEmptyItem = true
				}
				j++
			}
			delete(fm.Fields, key)
			fm.Lists[key] = items
			if sawEmptyItem {
				fm.EmptyListKeys[key] = true
			}
			i = j
		default:
			delete(fm.Lists, key)
			fm.Fields[key] = stripQuotes(value)
			i++
		}
	}
	return fm
}

// NormalizeID normalizes any ADR id spelling (`339`, `ADR-339`) to `000339`.
func NormalizeID(raw string) string {
	digits := nonDigitRE.ReplaceAllString(raw, "")
	for len(digits) < 6 {
		digits = "0" + digits
	}
	return digits
}

// LoadDir loads every NNNNNN.md file under dir into id -> ADR.
func LoadDir(dir string) (map[string]ADR, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	adrs := map[string]ADR{}
	for _, entry := range entries {
		name := entry.Name()
		stem := strings.TrimSuffix(name, ".md")
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || !adrStemRE.MatchString(stem) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		fm := ParseFrontmatter(string(content))
		rawSupersedes := fm.Lists["supersedes"]
		if rawSupersedes == nil {
			// adr_graph.py wraps a scalar supersedes value into a one-element list
			if scalar, ok := fm.Fields["supersedes"]; ok && scalar != "" {
				rawSupersedes = []string{scalar}
			}
		}
		supersedes := make([]string, 0, len(rawSupersedes))
		for _, s := range rawSupersedes {
			supersedes = append(supersedes, NormalizeID(s))
		}
		adrs[stem] = ADR{
			ID:                  stem,
			Title:               fm.Fields["title"],
			Status:              fm.Fields["status"],
			Supersedes:          supersedes,
			EmptySupersedesStub: fm.EmptyListKeys["supersedes"],
		}
	}
	return adrs, nil
}

// SupersedesGraph builds new-id -> [old-id...] adjacency from loaded ADRs.
func SupersedesGraph(adrs map[string]ADR) map[string][]string {
	g := make(map[string][]string, len(adrs))
	for id, a := range adrs {
		g[id] = append([]string{}, a.Supersedes...)
	}
	return g
}

// SortedIDs returns all ADR ids in ascending order.
func SortedIDs(adrs map[string]ADR) []string {
	ids := make([]string, 0, len(adrs))
	for id := range adrs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
