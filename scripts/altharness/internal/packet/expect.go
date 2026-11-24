package packet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	sequenceTestFileRe   = regexp.MustCompile(`(?i)(?:^|/)(?:(?:tests?|__tests__|e2e|testdata)/.+|[^/]+_test\.go|test_[^/]+\.py|test-[^/]+\.py|[^/]+_test\.py|[^/]*test[^/]*\.sh|[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)|[^/]+_test\.[cm]?[jt]sx?|tests\.rs|[^/]+_tests?\.rs)$`)
	allowedLintCommentRe = regexp.MustCompile(`(?i)(?:#\s*(?:ruff:\s*)?noqa\b|(?://|#|/\*)\s*#?nosec\b|//\s*nolint\b|#!?\[\s*(?:allow|expect)\b|biome-ignore\b|deno-lint-ignore\b|\bexpires\s*:|^#!)`)
	testPathRe           = regexp.MustCompile(`(?i)(?:^|/)(?:tests?/|e2e/|__tests__/|test_[^/]+\.py$|test-[^/]+\.py$|tests\.rs$|[^/]+_tests?\.rs$|[^/]+_test\.(?:go|py|[cm]?[jt]sx?)$|[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)$)`)
	expectGoRe           = regexp.MustCompile(`\b(?:require|assert)\.\w+\s*\(|\bt\.(?:Fatal|Fatalf|Fatalln|Error|Errorf)\s*\(`)
	expectPyRe           = regexp.MustCompile(`(?:^|\n)\s*assert\b|\bself\.assert\w+\s*\(|\bpytest\.raises\s*\(`)
	expectRsRe           = regexp.MustCompile(`\b(?:debug_|prop_)?assert(?:_eq|_ne|_matches)?!\s*[\(\[{]`)
	expectTsRe           = regexp.MustCompile(`\bexpect(?:\.\w+)?\s*\(`)
	chainMemberRe        = regexp.MustCompile(`\s*\??\.\s*[A-Za-z_$][A-Za-z0-9_$]*`)
	rustTestAttrAnyRe    = regexp.MustCompile(`#\[\s*(?:cfg\(\s*test\s*\)|(?:[\w:]+::)?test\b|rstest\b)`)
)

type behaviorCandidate struct {
	id, file, function, kind, before, after string
}

func stableCandidateID(filePath, function, kind string) string {
	sum := sha256.Sum256([]byte(filePath + "|" + function + "|" + kind))
	return kind + ":" + hex.EncodeToString(sum[:])[:12]
}

func isTestFile(logical, language, before string) bool {
	norm := strings.ReplaceAll(logical, `\`, "/")
	if testPathRe.MatchString(norm) {
		return true
	}
	return language == "rust" && rustTestAttrAnyRe.MatchString(before)
}

func stripCCommentsKeepStrings(src, language string) string {
	var out strings.Builder
	i := 0
	n := len(src)
	nested := language == "rust"
	for i < n {
		if end, ok := scanStringLiteralEnd(src, i, language); ok {
			out.WriteString(src[i:end])
			i = end
			continue
		}
		ch := src[i]
		var nxt byte
		if i+1 < n {
			nxt = src[i+1]
		}
		if ch == '/' && nxt == '/' {
			nl := strings.IndexByte(src[i+2:], '\n')
			if nl < 0 {
				break
			}
			i = i + 2 + nl
			continue
		}
		if ch == '/' && nxt == '*' {
			end := scanBlockCommentEnd(src, i, nested)
			out.WriteByte(' ')
			for _, c := range src[i:end] {
				if c == '\n' {
					out.WriteByte('\n')
				}
			}
			i = end
			continue
		}
		out.WriteByte(ch)
		i++
	}
	return out.String()
}

func findBalancedClose(src string, openIdx int, language string) int {
	if openIdx < 0 || openIdx >= len(src) {
		return len(src)
	}
	pairs := map[byte]byte{'(': ')', '[': ']', '{': '}'}
	closers := map[byte]struct{}{')': {}, ']': {}, '}': {}}
	stack := []byte{pairs[src[openIdx]]}
	idx := openIdx + 1
	for idx < len(src) && len(stack) > 0 {
		if end, ok := scanStringLiteralEnd(src, idx, language); ok {
			idx = end
			continue
		}
		ch := src[idx]
		if close, ok := pairs[ch]; ok {
			stack = append(stack, close)
		} else if _, isClose := closers[ch]; isClose && stack[len(stack)-1] == ch {
			stack = stack[:len(stack)-1]
		}
		idx++
	}
	return idx
}

func consumeChainedCalls(src string, pos int, language string) int {
	for pos < len(src) {
		loc := chainMemberRe.FindStringIndex(src[pos:])
		if loc == nil || loc[0] != 0 {
			break
		}
		pos += loc[1]
		scan := pos
		for scan < len(src) && isASCIISpace(src[scan]) {
			scan++
		}
		if scan < len(src) && (src[scan] == '(' || src[scan] == '[') {
			pos = findBalancedClose(src, scan, language)
			continue
		}
		pos = scan
		break
	}
	return pos
}

func normalizeCodeSegment(seg string) string {
	collapsed := regexp.MustCompile(`\s+`).ReplaceAllString(seg, " ")
	collapsed = regexp.MustCompile(`\s+([)\],.;])`).ReplaceAllString(collapsed, "$1")
	collapsed = regexp.MustCompile(`([(\[.,])\s+`).ReplaceAllString(collapsed, "$1")
	collapsed = regexp.MustCompile(`,([)\]])`).ReplaceAllString(collapsed, "$1")
	return collapsed
}

func normalizeExpectationSpan(span, language string) string {
	var parts []string
	var buf strings.Builder
	i := 0
	for i < len(span) {
		if end, ok := scanStringLiteralEnd(span, i, language); ok {
			if buf.Len() > 0 {
				parts = append(parts, normalizeCodeSegment(buf.String()))
				buf.Reset()
			}
			parts = append(parts, span[i:end])
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

func stringRanges(src, language string) [][2]int {
	var ranges [][2]int
	i := 0
	for i < len(src) {
		if end, ok := scanStringLiteralEnd(src, i, language); ok {
			ranges = append(ranges, [2]int{i, end})
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

func expectationPattern(language string) *regexp.Regexp {
	switch language {
	case "go":
		return expectGoRe
	case "python":
		return expectPyRe
	case "rust":
		return expectRsRe
	case "typescript":
		return expectTsRe
	default:
		return nil
	}
}

func extractExpectationCounts(src, language string) map[string]int {
	counts := map[string]int{}
	pattern := expectationPattern(language)
	if pattern == nil {
		return counts
	}
	clean := src
	if language != "python" {
		clean = stripCCommentsKeepStrings(src, language)
	}
	spans := stringRanges(clean, language)
	pos := 0
	for pos < len(clean) {
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
		endIdx := endMatch
		if openIdx >= 0 && openIdx < len(clean) && (clean[openIdx] == '(' || clean[openIdx] == '[' || clean[openIdx] == '{') {
			endIdx = findBalancedClose(clean, openIdx, language)
			if language == "typescript" || language == "go" {
				endIdx = consumeChainedCalls(clean, endIdx, language)
			}
		} else if language == "python" {
			nl := strings.IndexByte(clean[start:], '\n')
			if nl < 0 {
				endIdx = len(clean)
			} else {
				endIdx = start + nl
			}
		}
		span := clean[start:endIdx]
		normalized := normalizeExpectationSpan(span, language)
		if normalized != "" {
			counts[normalized]++
		}
		if endIdx > endMatch {
			pos = endIdx
		} else {
			pos = endMatch
		}
	}
	return counts
}

func screenTestExpectations(before, after, logical, language string) []behaviorCandidate {
	if !isTestFile(logical, language, before) {
		return nil
	}
	bExp := extractExpectationCounts(before, language)
	aExp := extractExpectationCounts(after, language)
	var removed, added []string
	for k, n := range bExp {
		if n > aExp[k] {
			removed = append(removed, k)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	for k, n := range aExp {
		if n > bExp[k] {
			added = append(added, k)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	beforeSummary := strings.Join(removed, "; ")
	if len(beforeSummary) > 240 {
		beforeSummary = beforeSummary[:240]
	}
	afterSummary := strings.Join(added, "; ")
	if afterSummary == "" {
		afterSummary = "<removed>"
	}
	if len(afterSummary) > 240 {
		afterSummary = afterSummary[:240]
	}
	kind := "test.expectation_change"
	return []behaviorCandidate{{
		id:       stableCandidateID(logical, "<test>", kind),
		file:     logical,
		function: "<test>",
		kind:     kind,
		before:   beforeSummary,
		after:    afterSummary,
	}}
}

func pyQuoteList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%q", item))
	}
	b.WriteByte(']')
	return b.String()
}
