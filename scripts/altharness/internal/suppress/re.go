package suppress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	adrIDRe    = regexp.MustCompile(`\bADR-(\d{6})\b`)
	adrExactRe = regexp.MustCompile(`^ADR-(\d{6})$`)
	adrExactRE = adrExactRe
	adrPathRE  = regexp.MustCompile(`^docs/ADR/(\d{6})\.md$`)
	allZerosRE = regexp.MustCompile(`^0+$`)

	expiresInlineRE = regexp.MustCompile(`(?i)\bexpires\s*[:=]\s*([^\s),;\]]+)`)
	expiresDateRe   = regexp.MustCompile(`(?i)\bexpires\s*[:=]\s*(\d{4}-\d{2}-\d{2})\b`)
	nonReasonRE     = regexp.MustCompile(`[\s#/*<>!:.,;()\[\]{}—\-]+`)
	generatedRE     = regexp.MustCompile(`(?i)Code generated .* DO NOT EDIT\.?`)

	pyFileNoqaRe     = regexp.MustCompile(`(?i)^#\s*(?:ruff|flake8)\s*:\s*noqa\b(.*)$`)
	pyFileNoqaRE     = pyFileNoqaRe
	pyInlineNoqaRe   = regexp.MustCompile(`(?i)#\s*noqa\b(.*)$`)
	pyInlineNoqaRE   = pyInlineNoqaRe
	pyNoqaCodesRe    = regexp.MustCompile(`^:\s*([A-Z]+[0-9]+(?:\s*,\s*[A-Z]+[0-9]+)*)(?:$|[\s#\-—(;:])`)
	pyNoqaCodesRE    = pyNoqaCodesRe
	pyTrailingBadRE  = regexp.MustCompile(`(?i),\s*(?:ALL|\*)`)
	pyNosecRe        = regexp.MustCompile(`(?i)#\s*nosec\b(.*)$`)
	pyNosecRE        = pyNosecRe
	pyNosecCodesRE   = regexp.MustCompile(`(?i)^[\s:]*((?:B\d{3})(?:[\s,]+B\d{3})*)(?:$|[\s#\-—(;:])`)
	pySecondaryRe    = regexp.MustCompile(`(?i)#\s*(?:nosec|type:\s*ignore|pyright:\s*ignore|pyrefly:\s*ignore)\b.*$`)
	pySecondaryRE    = pySecondaryRe
	pyNosecSecondary = regexp.MustCompile(`(?i)#\s*(?:noqa|type:\s*ignore|pyright:\s*ignore|pyrefly:\s*ignore)\b.*$`)
	noqaLeadRE       = regexp.MustCompile(`^(?:--|—|-|#|:|;|\s)+`)
	reasonPrefixRe   = noqaLeadRE
	bCodeRE          = regexp.MustCompile(`(?i)B\d{3}`)

	sqlValueRE     = regexp.MustCompile(`(?i)(?:\bvalue\b|値)`)
	sqlUntrustedRE = regexp.MustCompile(`(?i)(?:\buntrusted\b|\bexternal\b|信頼境界外)`)
	sqlKindRE      = regexp.MustCompile(`(?i)(?:\bidentifier\b|識別子|\bnot[-_\s]?sql\b|非\s*SQL)`)
	sqlTrustRE     = regexp.MustCompile(`(?i)(?:\btrusted\b|\binternal\b|\bconstant\b|\ballowlist\b|信頼境界内)`)

	goNosecRe         = regexp.MustCompile(`(?s)^(?://|/\*)\s*#nosec\b(.*)$`)
	goNosecRE         = goNosecRe
	goNosecRulesRe    = regexp.MustCompile(`(?s)^((?:G\d{3}(?:[\s,]+)?)+)(?:--|//)(.*)$`)
	goNosecRulesRE    = goNosecRulesRe
	goNosecRuleOnlyRE = regexp.MustCompile(`^((?:G\d{3}(?:[\s,]+)?)+)(?:\*/)?$`)
	gCodeRe           = regexp.MustCompile(`G\d{3}`)
	gCodeRE           = gCodeRe

	rustInnerAllowRE    = regexp.MustCompile(`(?s)^#\s*!\s*\[\s*(?:allow|expect)\s*\(`)
	rustOuterAllowRE    = regexp.MustCompile(`(?s)^#\s*\[\s*(?:allow|expect)\s*\(`)
	rustInnerAllowCapRe = regexp.MustCompile(`(?s)^#\s*!\s*\[\s*(?:allow|expect)\s*\((.*)\)\s*\]$`)
	rustOuterAllowCapRe = regexp.MustCompile(`(?s)^#\s*\[\s*(?:allow|expect)\s*\((.*)\)\s*\]$`)

	tsBiomeSpecRe = regexp.MustCompile(`(?s)^([A-Za-z][A-Za-z0-9/_-]*)(?::(.*))?$`)
	tsBiomeSpecRE = tsBiomeSpecRe
	tsDenoRuleRE  = regexp.MustCompile(`^([a-z][a-z0-9-]*)`)
)

var (
	goNolintRe = reFn(func(s string) []string {
		linters, rest, ok := matchGoNolint(s)
		if !ok {
			return nil
		}
		return []string{s, linters, rest}
	})
	tsBiomeAllRe = reFn(func(s string) []string {
		rest, ok := directiveRest(s, "biome-ignore-all")
		if !ok {
			return nil
		}
		return []string{s, rest}
	})
	tsDenoFileRe = reFn(func(s string) []string {
		rest, ok := directiveRest(s, "deno-lint-ignore-file")
		if !ok {
			return nil
		}
		return []string{s, rest}
	})
	tsEslintBroadRe = reFn(func(s string) []string {
		rest, ok := directiveRest(s, "eslint-disable", "-next-line", "-line")
		if !ok {
			return nil
		}
		return []string{s, rest}
	})
	tsBiomeLineRe = reFn(func(s string) []string {
		rest, ok := directiveRest(s, "biome-ignore", "-all")
		if !ok {
			return nil
		}
		return []string{s, rest}
	})
	tsDenoLineRe = reFn(func(s string) []string {
		rest, ok := directiveRest(s, "deno-lint-ignore", "-file")
		if !ok {
			return nil
		}
		return []string{s, rest}
	})
)

type reFn func(string) []string

func (f reFn) FindStringSubmatch(s string) []string { return f(s) }

var sqlSuppressionCodes = map[string]struct{}{
	"S608": {}, "G201": {}, "G202": {}, "B608": {},
}

var legacyRuffKeys = []string{"ignore", "extend-ignore", "per-file-ignores", "extend-per-file-ignores"}

var tsExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"}

var sourceExts = []string{".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"}

type noqaKey struct {
	Path  string
	Codes string
	SHA   string
}

func pyQuote(s string) string { return pyReprString(s) }

func pyList(items []string) string { return pyListRepr(items) }

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func stringList(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

func stringListLoose(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, pyStr(item))
	}
	return out
}

func diffSorted(have, allowed map[string]struct{}) []string {
	var extra []string
	for k := range have {
		if _, ok := allowed[k]; !ok {
			extra = append(extra, k)
		}
	}
	sortStrings(extra)
	return extra
}

func isTestPathPattern(pattern string) bool {
	pat := strings.TrimLeft(strings.TrimSpace(pattern), "./")
	if pat == "" || strings.ContainsAny(pat, "{},") {
		return false
	}
	var segments []string
	for _, seg := range strings.Split(pat, "/") {
		if seg != "" {
			segments = append(segments, seg)
		}
	}
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if seg == "tests" {
			return true
		}
	}
	filename := segments[len(segments)-1]
	if !strings.HasSuffix(filename, ".py") {
		return false
	}
	return strings.HasPrefix(filename, "test_") || strings.HasSuffix(filename, "_test.py")
}

func hasExt(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func hasSuffixAny(path string, exts []string) bool { return hasExt(path, exts) }

func isExcludedSourcePath(relPath string) bool {
	norm := lstripDotSlash(relPath)
	if strings.HasPrefix(norm, "stopped-using-k8s/") || strings.HasPrefix(norm, "scripts/harness/corpus/") {
		return true
	}
	if strings.HasSuffix(norm, ".pb.go") || strings.HasSuffix(norm, ".connect.go") ||
		strings.HasSuffix(norm, "server.gen.go") || strings.HasSuffix(norm, ".md") {
		return true
	}
	if strings.HasPrefix(norm, "gen/") || strings.HasPrefix(norm, "alt-frontend-sv/src/lib/gen/") || strings.Contains(norm, "/gen/") {
		return true
	}
	return !hasExt(norm, sourceExts)
}

func isGeneratedSource(text string) bool {
	lines := splitLines(text)
	if len(lines) > generatedHeaderLines {
		lines = lines[:generatedHeaderLines]
	}
	for _, line := range lines {
		if generatedRE.MatchString(line) {
			return true
		}
	}
	return false
}

func isGeneratedSourceText(text string) bool { return isGeneratedSource(text) }

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range splitLines(text) {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func reasonLen(text string) int {
	cleaned := adrIDRe.ReplaceAllString(text, "")
	cleaned = expiresInlineRE.ReplaceAllString(cleaned, "")
	cleaned = nonReasonRE.ReplaceAllString(cleaned, "")
	return utf8.RuneCountInString(cleaned)
}

func hasSufficientReason(text string) bool { return nonSpaceLen(text) >= minReasonLength }

func stripCommentDelimiters(raw string) string {
	text := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(text, "<!--"):
		text = strings.TrimPrefix(text, "<!--")
		text = strings.TrimSuffix(text, "-->")
	case strings.HasPrefix(text, "/*"):
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSuffix(text, "*/")
	case strings.HasPrefix(text, "//"):
		text = strings.TrimPrefix(text, "//")
	case strings.HasPrefix(text, "#"):
		text = strings.TrimPrefix(text, "#")
	}
	return strings.TrimSpace(text)
}

func precedingComments(byLine map[int]string, start int) string {
	var parts []string
	for cursor := start - 1; ; cursor-- {
		s, ok := byLine[cursor]
		if !ok {
			break
		}
		parts = append(parts, s)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "\n")
}

func precedingCommentText(byLine map[int]string, start int) string {
	return precedingComments(byLine, start)
}

func pythonComments(text string) []lineComment { return scanPyComments(text) }

func adrIDs(text string) []string {
	matches := adrIDRe.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func extractExpiresLoose(text string) []time.Time {
	var dates []time.Time
	for _, m := range expiresInlineRE.FindAllStringSubmatch(text, -1) {
		token := strings.TrimRight(m[1], "),.;]}")
		if d, ok := parseISODate(token); ok {
			dates = append(dates, d)
		}
	}
	return dates
}

func lineFingerprintSHA256(line string) string {
	sum := sha256.Sum256([]byte(normalizeSourceLine(line)))
	return hex.EncodeToString(sum[:])
}

func defaultRepoRoot() string { return defaultRepo(context.Background()) }

func trimLeftSpace(s string) string { return strings.TrimLeftFunc(s, unicode.IsSpace) }

func skipSpaces(s string, i int) int {
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return i
}

func isLinterByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func startsWithNolintBoundary(s string) bool {
	if s == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r) || r == '*' || r == '/'
}

func consumeLinterList(s string) (raw, rest string, ok bool) {
	if s == "" || !isLinterByte(s[0]) {
		return "", s, false
	}
	i := 0
	for i < len(s) && isLinterByte(s[i]) {
		i++
	}
	for {
		j := skipSpaces(s, i)
		if j < len(s) && s[j] == ',' {
			k := skipSpaces(s, j+1)
			if k < len(s) && isLinterByte(s[k]) {
				for k < len(s) && isLinterByte(s[k]) {
					k++
				}
				i = k
				continue
			}
		}
		break
	}
	raw = s[:i]
	j := skipSpaces(s, i)
	if j < len(s) && s[j] == ',' {
		j++
	}
	return raw, s[j:], true
}

func matchGoNolint(raw string) (lintersRaw, rest string, ok bool) {
	s := raw
	switch {
	case strings.HasPrefix(s, "//"):
		s = s[2:]
	case strings.HasPrefix(s, "/*"):
		s = s[2:]
	default:
		return "", "", false
	}
	s = trimLeftSpace(s)
	if !strings.HasPrefix(s, "nolint") {
		return "", "", false
	}
	after := s[len("nolint"):]
	if strings.HasPrefix(after, ":") {
		names, next, good := consumeLinterList(trimLeftSpace(after[1:]))
		if good && startsWithNolintBoundary(next) {
			return names, next, true
		}
	}
	if startsWithNolintBoundary(after) {
		return "", after, true
	}
	return "", "", false
}

func isASCIIWord(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func indexDirective(s, word string, forbidden ...string) int {
	if len(word) == 0 || len(s) < len(word) {
		return -1
	}
	for start := 0; start <= len(s)-len(word); {
		i := strings.Index(s[start:], word)
		if i < 0 {
			return -1
		}
		i += start
		end := i + len(word)
		prevOK := i == 0 || !isASCIIWord(s[i-1])
		nextOK := end == len(s) || !isASCIIWord(s[end])
		if prevOK && nextOK {
			rest := s[end:]
			bad := false
			for _, suf := range forbidden {
				if !strings.HasPrefix(rest, suf) {
					continue
				}
				after := end + len(suf)
				if after == len(s) || !isASCIIWord(s[after]) {
					bad = true
					break
				}
			}
			if !bad {
				return i
			}
		}
		start = i + 1
	}
	return -1
}

func directiveRest(body, word string, forbidden ...string) (string, bool) {
	i := indexDirective(body, word, forbidden...)
	if i < 0 {
		return "", false
	}
	return body[i+len(word):], true
}
