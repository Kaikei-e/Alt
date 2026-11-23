package screener

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	testPathRE         = regexp.MustCompile(`(?i)(?:^|/)(?:tests?/|e2e/|__tests__/|test_[^/]+\.py$|test-[^/]+\.py$|tests\.rs$|[^/]+_tests?\.rs$|[^/]+_test\.(?:go|py|[cm]?[jt]sx?)$|[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)$)`)
	rustTestAttrRE     = regexp.MustCompile(`#\[\s*(?:cfg\(\s*test\s*\)|(?:[\w:]+::)?test\b|rstest\b)`)
	expectGoRE         = regexp.MustCompile(`\b(?:require|assert)\.\w+\s*\(|\bt\.(?:Fatal|Fatalf|Fatalln|Error|Errorf)\s*\(`)
	expectRustRE       = regexp.MustCompile(`\b(?:debug_|prop_)?assert(?:_eq|_ne|_matches)?!\s*[\(\[\{]`)
	expectTSRE         = regexp.MustCompile(`\bexpect(?:\.\w+)?\s*\(`)
	chainMemberRE      = regexp.MustCompile(`\s*\??\.\s*[A-Za-z_$][A-Za-z0-9_$]*`)
	wsRunRE            = regexp.MustCompile(`\s+`)
	wsBeforePunctRE    = regexp.MustCompile(`\s+([)\],.;])`)
	wsAfterPunctRE     = regexp.MustCompile(`([(\[.,])\s+`)
	commaBeforeCloseRE = regexp.MustCompile(`,([)\]])`)
)

func isTestFile(logical, language, beforeSrc string) bool {
	norm := strings.ReplaceAll(logical, "\\", "/")
	if testPathRE.MatchString(norm) {
		return true
	}
	return language == "rust" && rustTestAttrRE.MatchString(beforeSrc)
}

func screenTestExpectations(beforeSrc, afterSrc, logical, language string) ([]Candidate, error) {
	if !isTestFile(logical, language, beforeSrc) {
		return nil, nil
	}
	beforeCounts, err := expectationCounts(beforeSrc, language)
	if err != nil {
		return nil, err
	}
	afterCounts, err := expectationCounts(afterSrc, language)
	if err != nil {
		return nil, err
	}
	removed := subCounts(beforeCounts, afterCounts)
	if len(removed) == 0 {
		return nil, nil
	}
	added := subCounts(afterCounts, beforeCounts)
	beforeSummary := truncateRunes(joinSortedKeys(removed), 240)
	afterSummary := joinSortedKeys(added)
	if afterSummary == "" {
		afterSummary = "<removed>"
	}
	afterSummary = truncateRunes(afterSummary, 240)
	return []Candidate{makeCandidate(logical, "<test>", "test.expectation_change", beforeSummary, afterSummary)}, nil
}

func expectationCounts(src, language string) (map[string]int, error) {
	if language == "python" {
		stmts, err := parsePython(src)
		if err != nil {
			return nil, err
		}
		counts := map[string]int{}
		walkPyExpect(stmts, counts)
		return counts, nil
	}
	pattern := expectPattern(language)
	if pattern == nil {
		return map[string]int{}, nil
	}
	clean := stripCommentsKeepStrings(src, language)
	spans := stringRanges(clean, language)
	counts := map[string]int{}
	pos := 0
	n := len(clean)
	for pos < n {
		loc := pattern.FindStringIndex(clean[pos:])
		if loc == nil {
			break
		}
		start := pos + loc[0]
		endMatch := pos + loc[1]
		if insideRange(start, spans) {
			pos = endMatch
			continue
		}
		openIdx := endMatch - 1
		endIdx := findBalancedClose(clean, openIdx, language)
		if language == "typescript" || language == "go" {
			endIdx = consumeChainedCalls(clean, endIdx, language)
		}
		if endIdx < start {
			endIdx = start
		}
		if endIdx > len(clean) {
			endIdx = len(clean)
		}
		normalized := normalizeExpectationSpan(clean[start:endIdx], language)
		if normalized != "" {
			counts[normalized]++
		}
		if endIdx > endMatch {
			pos = endIdx
		} else {
			pos = endMatch
		}
	}
	return counts, nil
}

func expectPattern(language string) *regexp.Regexp {
	switch language {
	case "go":
		return expectGoRE
	case "rust":
		return expectRustRE
	case "typescript":
		return expectTSRE
	default:
		return nil
	}
}

func subCounts(a, b map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range a {
		if d := v - b[k]; d > 0 {
			out[k] = d
		}
	}
	return out
}

func joinSortedKeys(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return strings.Join(keys, "; ")
}

func sortStrings(items []string) {
	// local wrapper so expect.go does not depend on a second import alias
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j] < items[j-1] {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for count := 0; count < n && i < len(s); count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

func stringRanges(src, language string) [][2]int {
	var ranges [][2]int
	i := 0
	n := len(src)
	for i < n {
		end, ok := scanStringLiteralEnd(src, i, language)
		if ok {
			ranges = append(ranges, [2]int{i, end})
			if end <= i {
				i++
				continue
			}
			i = end
			continue
		}
		i++
	}
	return ranges
}

func insideRange(pos int, ranges [][2]int) bool {
	for _, r := range ranges {
		if r[0] <= pos && pos < r[1] {
			return true
		}
	}
	return false
}

func findBalancedClose(src string, openIdx int, language string) int {
	if openIdx < 0 || openIdx >= len(src) {
		return len(src)
	}
	closer, ok := openToCloseByte(src[openIdx])
	if !ok {
		return openIdx + 1
	}
	stack := []byte{closer}
	idx := openIdx + 1
	n := len(src)
	for idx < n && len(stack) > 0 {
		if end, isStr := scanStringLiteralEnd(src, idx, language); isStr {
			idx = end
			continue
		}
		ch := src[idx]
		if c, isOpen := openToCloseByte(ch); isOpen {
			stack = append(stack, c)
		} else if (ch == ')' || ch == ']' || ch == '}') && stack[len(stack)-1] == ch {
			stack = stack[:len(stack)-1]
		}
		idx++
	}
	return idx
}

func openToCloseByte(ch byte) (byte, bool) {
	switch ch {
	case '(':
		return ')', true
	case '[':
		return ']', true
	case '{':
		return '}', true
	default:
		return 0, false
	}
}

func consumeChainedCalls(src string, pos int, language string) int {
	n := len(src)
	for pos < n {
		loc := chainMemberRE.FindStringIndex(src[pos:])
		if loc == nil || loc[0] != 0 {
			break
		}
		pos += loc[1]
		scan := pos
		for scan < n && isASCIISpace(src[scan]) {
			scan++
		}
		if scan < n && (src[scan] == '(' || src[scan] == '[') {
			pos = findBalancedClose(src, scan, language)
		}
	}
	return pos
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

func normalizeCodeSegment(seg string) string {
	collapsed := wsRunRE.ReplaceAllString(seg, " ")
	collapsed = wsBeforePunctRE.ReplaceAllString(collapsed, "$1")
	collapsed = wsAfterPunctRE.ReplaceAllString(collapsed, "$1")
	return commaBeforeCloseRE.ReplaceAllString(collapsed, "$1")
}

func normalizeExpectationSpan(span, language string) string {
	var parts []string
	var buf strings.Builder
	i := 0
	n := len(span)
	for i < n {
		if end, ok := scanStringLiteralEnd(span, i, language); ok {
			if buf.Len() > 0 {
				parts = append(parts, normalizeCodeSegment(buf.String()))
				buf.Reset()
			}
			parts = append(parts, span[i:end])
			if end <= i {
				i++
				continue
			}
			i = end
			continue
		}
		buf.WriteByte(span[i])
		i++
	}
	if buf.Len() > 0 {
		parts = append(parts, normalizeCodeSegment(buf.String()))
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func walkPyExpect(stmts []pyStmt, counts map[string]int) {
	for _, s := range stmts {
		if s.kind == skAssert {
			counts[unparseAssert(s)]++
		}
		walkExprExpect(s.test, counts)
		walkExprExpect(s.value, counts)
		walkExprExpect(s.ann, counts)
		for _, e := range s.withExprs {
			walkExprExpect(e, counts)
		}
		for _, h := range s.handlers {
			walkPyExpect(h.body, counts)
		}
		for _, cs := range s.cases {
			walkExprExpect(cs.guard, counts)
			walkPyExpect(cs.body, counts)
		}
		walkPyExpect(s.body, counts)
		walkPyExpect(s.orelse, counts)
		walkPyExpect(s.final, counts)
	}
}

func walkExprExpect(e *px, counts map[string]int) {
	if e == nil || e.op == "lambda" {
		return
	}
	if e.op == "call" && len(e.kids) > 0 && isPyAssertCall(e.kids[0]) {
		counts[unparse(e)]++
	}
	for _, k := range e.kids {
		walkExprExpect(k, counts)
	}
}

func isPyAssertCall(fn *px) bool {
	if fn == nil || fn.op != "attr" || len(fn.kids) == 0 || fn.kids[0] == nil || fn.kids[0].op != "name" {
		return false
	}
	base := fn.kids[0].lit
	if base == "self" && strings.HasPrefix(fn.lit, "assert") {
		return true
	}
	return base == "pytest" && fn.lit == "raises"
}

func unparseAssert(s pyStmt) string {
	text := "assert " + unparse(s.test)
	if s.value != nil {
		text += ", " + unparse(s.value)
	}
	return text
}
