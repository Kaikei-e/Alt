package screener

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	rustOptionRegexRE   = regexp.MustCompile(`\bOption\s*<\s*[^>]*\bRegex\b[^>]*>`)
	rustDotOKRE         = regexp.MustCompile(`\.\s*ok\s*\(\s*\)`)
	rustUnwrapDefaultRE = regexp.MustCompile(`\.\s*unwrap_or_default\s*\(\s*\)`)
	rustCfgTestRE       = regexp.MustCompile(`#\[\s*cfg\(\s*test\s*\)\s*\]`)
	rustFnDeclRE        = regexp.MustCompile(`\bfn\s+([A-Za-z_][A-Za-z0-9_]*)`)
	rustModDeclRE       = regexp.MustCompile(`\bmod\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	rustSwallowKeys     = []string{"Option<Regex>", ".ok()", ".unwrap_or_default()"}
)

func countRustSwallows(segment string) map[string]int {
	return map[string]int{
		"Option<Regex>":        len(rustOptionRegexRE.FindAllStringIndex(segment, -1)),
		".ok()":                len(rustDotOKRE.FindAllStringIndex(segment, -1)),
		".unwrap_or_default()": len(rustUnwrapDefaultRE.FindAllStringIndex(segment, -1)),
	}
}

func findRustBraceClose(src string, openIdx int) int {
	depth := 1
	idx := openIdx + 1
	n := len(src)
	for idx < n && depth > 0 {
		switch src[idx] {
		case '{':
			depth++
		case '}':
			depth--
		}
		idx++
	}
	return idx
}

func findRustFnBodyOpen(src string, start int) (int, bool) {
	paren, bracket := 0, 0
	n := len(src)
	for idx := start; idx < n; idx++ {
		switch src[idx] {
		case '(':
			paren++
		case ')':
			if paren > 0 {
				paren--
			}
		case '[':
			bracket++
		case ']':
			if bracket > 0 {
				bracket--
			}
		default:
			if paren == 0 && bracket == 0 {
				switch src[idx] {
				case '{':
					return idx, true
				case ';':
					return 0, false
				}
			}
		}
	}
	return 0, false
}

type rustRange struct {
	start, end int
	name       string
}

func findCfgTestRanges(cleaned string) []rustRange {
	var ranges []rustRange
	for _, loc := range rustCfgTestRE.FindAllStringIndex(cleaned, -1) {
		endAttr := loc[1]
		tailEnd := endAttr + 200
		if tailEnd > len(cleaned) {
			tailEnd = len(cleaned)
		}
		tail := cleaned[endAttr:tailEnd]
		modLoc := rustModDeclRE.FindStringSubmatchIndex(tail)
		if modLoc == nil {
			continue
		}
		openIdx := endAttr + modLoc[1] - 1
		if openIdx < 0 || openIdx >= len(cleaned) || cleaned[openIdx] != '{' {
			continue
		}
		closeIdx := findRustBraceClose(cleaned, openIdx)
		ranges = append(ranges, rustRange{start: openIdx, end: closeIdx, name: tail[modLoc[2]:modLoc[3]]})
	}
	return ranges
}

type rustSpan struct {
	start, end int
	key        string
}

func extractRustScopes(cleaned string) map[string]map[string]int {
	testRanges := findCfgTestRanges(cleaned)
	var fnSpans []rustSpan
	nameCounts := map[string]int{}
	for _, loc := range rustFnDeclRE.FindAllStringSubmatchIndex(cleaned, -1) {
		bodyOpen, ok := findRustFnBodyOpen(cleaned, loc[1])
		if !ok {
			continue
		}
		bodyClose := findRustBraceClose(cleaned, bodyOpen)
		fnName := cleaned[loc[2]:loc[3]]
		testMod := ""
		for _, r := range testRanges {
			if r.start <= loc[0] && loc[0] < r.end {
				testMod = r.name
				break
			}
		}
		base := fnName
		if testMod != "" {
			base = testMod + "::" + fnName
		}
		nameCounts[base]++
		key := base
		if nameCounts[base] > 1 {
			key = fmt.Sprintf("%s#%d", base, nameCounts[base])
		}
		fnSpans = append(fnSpans, rustSpan{start: loc[0], end: bodyClose, key: key})
	}
	scopes := map[string]map[string]int{}
	for _, sp := range fnSpans {
		scopes[sp.key] = countRustSwallows(cleaned[sp.start:sp.end])
	}
	sorted := append([]rustSpan(nil), fnSpans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	var prod, testParts []string
	pos := 0
	appendChunk := func(chunk string, at int) {
		inTest := false
		for _, r := range testRanges {
			if r.start <= at && at < r.end {
				inTest = true
				break
			}
		}
		if inTest {
			testParts = append(testParts, chunk)
		} else {
			prod = append(prod, chunk)
		}
	}
	for _, sp := range sorted {
		if sp.start > pos {
			appendChunk(cleaned[pos:sp.start], pos)
		}
		if sp.end > pos {
			pos = sp.end
		}
	}
	if pos < len(cleaned) {
		appendChunk(cleaned[pos:], pos)
	}
	scopes["<module>"] = countRustSwallows(strings.Join(prod, ""))
	if len(testParts) > 0 {
		scopes["<module:test>"] = countRustSwallows(strings.Join(testParts, ""))
	}
	return scopes
}

func screenRust(before, after, logical string) []Candidate {
	bScopes := extractRustScopes(stripCStyle(before, true))
	aScopes := extractRustScopes(stripCStyle(after, true))
	zero := map[string]int{"Option<Regex>": 0, ".ok()": 0, ".unwrap_or_default()": 0}
	names := make([]string, 0, len(aScopes))
	for name := range aScopes {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []Candidate
	for _, scope := range names {
		aCounts := aScopes[scope]
		bCounts := zero
		if existing, ok := bScopes[scope]; ok {
			bCounts = existing
		}
		increased := false
		for _, key := range rustSwallowKeys {
			if aCounts[key] > bCounts[key] {
				increased = true
				break
			}
		}
		if increased {
			out = append(out, makeCandidate(logical, scope, "rust.fallible_error_swallow", fmtCounter(bCounts), fmtCounter(aCounts)))
		}
	}
	return out
}
