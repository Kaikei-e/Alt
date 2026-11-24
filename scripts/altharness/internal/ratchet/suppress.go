package ratchet

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	generatedHeader = regexp.MustCompile(`(?i)Code generated .* DO NOT EDIT\.?`)
	pyFileNoqa      = regexp.MustCompile(`(?i)^#\s*(?:ruff|flake8)\s*:\s*noqa\b`)
	pyInlineNoqa    = regexp.MustCompile(`(?i)#\s*noqa\b`)
	pyNosec         = regexp.MustCompile(`(?i)#\s*nosec\b`)
	goNolint        = regexp.MustCompile(`^(?://|/\*)\s*nolint(?::|(?:$|[\s*/]))`)
	goNosec         = regexp.MustCompile(`^(?://|/\*)\s*#nosec\b`)
	rustAllow       = regexp.MustCompile(`^#\s*!?\s*\[\s*(?:allow|expect)\s*\(`)
	tsBiome         = regexp.MustCompile(`\bbiome-ignore\b`)
	tsDeno          = regexp.MustCompile(`\bdeno-lint-ignore\b`)
	gosecMsgRule    = regexp.MustCompile(`^(G\d{3}):`)
)

var sourceExts = []string{".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"}
var tsExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte"}

var markerNames = map[string]struct{}{
	"go.mod": {}, "Cargo.toml": {}, "pyproject.toml": {}, "deno.json": {}, "bun.lock": {}, "buf.yaml": {},
}

type prefixInfo struct{ isF bool }

var stringPrefixes map[string]prefixInfo

func init() {
	bases := []string{"", "b", "r", "u", "f", "br", "rb", "fr", "rf"}
	stringPrefixes = make(map[string]prefixInfo, 64)
	for _, b := range bases {
		n := len(b)
		for mask := 0; mask < 1<<n; mask++ {
			var sb strings.Builder
			for i := 0; i < n; i++ {
				c := rune(b[i])
				if mask&(1<<i) != 0 {
					c = unicode.ToUpper(c)
				}
				sb.WriteRune(c)
			}
			pref := sb.String()
			stringPrefixes[pref] = prefixInfo{isF: strings.ContainsAny(pref, "fF")}
		}
	}
}

func isExcludedSourcePath(rel string) bool {
	norm := trimLeftSet(rel, "./")
	if strings.HasPrefix(norm, "stopped-using-k8s/") || strings.HasPrefix(norm, "scripts/harness/corpus/") {
		return true
	}
	if strings.HasSuffix(norm, ".pb.go") || strings.HasSuffix(norm, ".connect.go") || strings.HasSuffix(norm, "server.gen.go") || strings.HasSuffix(norm, ".md") {
		return true
	}
	if strings.HasPrefix(norm, "gen/") || strings.HasPrefix(norm, "alt-frontend-sv/src/lib/gen/") || strings.Contains(norm, "/gen/") {
		return true
	}
	return !hasSuffix(norm, sourceExts)
}

func hasSuffix(s string, sufs []string) bool {
	for _, suf := range sufs {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

func isGeneratedSourceText(text string) bool {
	lines := splitLines(text)
	n := len(lines)
	if n > 40 {
		n = 40
	}
	for i := 0; i < n; i++ {
		if generatedHeader.MatchString(lines[i]) {
			return true
		}
	}
	return false
}

func normalizeSourceLine(line string) string {
	return strings.Join(strings.Fields(line), " ")
}

func countFileSuppressions(relPath, text string) map[string]int {
	if isExcludedSourcePath(relPath) || isGeneratedSourceText(text) {
		return nil
	}
	counts := map[string]int{}
	switch {
	case strings.HasSuffix(relPath, ".py"):
		noqa, nosec := countPythonSuppressions(text)
		if noqa > 0 {
			counts["python_noqa"] = noqa
		}
		if nosec > 0 {
			counts["python_nosec"] = nosec
		}
	case strings.HasSuffix(relPath, ".go"):
		comments := scanGoComments(text)
		nolint, nosec := 0, 0
		for _, raw := range comments {
			if goNolint.MatchString(raw) {
				nolint++
			}
			if goNosec.MatchString(raw) {
				nosec++
			}
		}
		if nolint > 0 {
			counts["go_nolint"] = nolint
		}
		if nosec > 0 {
			counts["go_nosec"] = nosec
		}
	case strings.HasSuffix(relPath, ".rs"):
		if n := countRustAllows(text); n > 0 {
			counts["rust_allow"] = n
		}
	case hasSuffix(relPath, tsExts):
		comments := scanTSComments(text, strings.HasSuffix(relPath, ".svelte"))
		biome, deno := 0, 0
		for _, raw := range comments {
			if biomeIgnore(raw) {
				biome++
			}
			if denoIgnore(raw) {
				deno++
			}
		}
		if biome > 0 {
			counts["ts_biome_ignore"] = biome
		}
		if deno > 0 {
			counts["ts_deno_ignore"] = deno
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func biomeIgnore(raw string) bool {
	locs := tsBiome.FindAllStringIndex(raw, -1)
	for _, loc := range locs {
		rest := raw[loc[1]:]
		if strings.HasPrefix(rest, "-all") {
			continue
		}
		return true
	}
	return false
}

func denoIgnore(raw string) bool {
	locs := tsDeno.FindAllStringIndex(raw, -1)
	for _, loc := range locs {
		rest := raw[loc[1]:]
		if strings.HasPrefix(rest, "-file") {
			continue
		}
		return true
	}
	return false
}

func countPythonSuppressions(text string) (int, int) {
	comments, failed := pythonComments(text)
	if failed {
		comments = fallbackComments(text)
	}
	noqa, nosec := 0, 0
	for _, raw := range comments {
		if !pyFileNoqa.MatchString(raw) && pyInlineNoqa.MatchString(raw) {
			noqa++
		}
		if pyNosec.MatchString(raw) {
			nosec++
		}
	}
	return noqa, nosec
}

func fallbackComments(text string) []string {
	var comments []string
	for _, line := range splitLines(text) {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if strings.HasPrefix(trimmed, "#") {
			comments = append(comments, trimmed)
		}
	}
	return comments
}

type pyscan struct {
	s        string
	i        int
	comments []string
	failed   bool
	triple   bool
}

func pythonComments(text string) ([]string, bool) {
	p := &pyscan{s: text}
	depth := 0
	for p.i < len(p.s) && !p.failed {
		if p.tryString() {
			continue
		}
		c := p.s[p.i]
		switch c {
		case '\\':
			if !p.consumeLineCont() {
				p.failed = true
			}
		case '#':
			p.consumeComment()
		case '(', '[', '{':
			depth++
			p.i++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			p.i++
		default:
			p.i++
		}
	}
	if p.failed || depth != 0 {
		return nil, true
	}
	return p.comments, false
}

func (p *pyscan) consumeComment() {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] != '\n' && p.s[p.i] != '\r' {
		p.i++
	}
	p.comments = append(p.comments, p.s[start:p.i])
}

func (p *pyscan) consumeLineCont() bool {
	if p.i+1 >= len(p.s) {
		return false
	}
	if p.s[p.i+1] == '\n' {
		p.i += 2
		return true
	}
	if p.s[p.i+1] == '\r' {
		p.i += 2
		if p.i < len(p.s) && p.s[p.i] == '\n' {
			p.i++
		}
		return true
	}
	return false
}

func (p *pyscan) tryString() bool {
	rest := p.s[p.i:]
	for n := 2; n >= 0; n-- {
		if n > len(rest) {
			continue
		}
		pref := rest[:n]
		if n > 0 {
			if _, ok := stringPrefixes[pref]; !ok {
				continue
			}
		}
		q := rest[n:]
		if q == "" || (q[0] != '\'' && q[0] != '"') {
			continue
		}
		quote := q[:1]
		if len(q) >= 3 && q[1] == q[0] && q[2] == q[0] {
			quote = q[:3]
		}
		info := stringPrefixes[pref]
		p.i += n + len(quote)
		if info.isF {
			p.scanFString(quote)
		} else {
			p.scanPlain(quote)
		}
		return true
	}
	return false
}

func (p *pyscan) scanPlain(quote string) {
	triple := len(quote) == 3
	for p.i < len(p.s) {
		if !triple && (p.s[p.i] == '\n' || p.s[p.i] == '\r') {
			p.failed = true
			return
		}
		if strings.HasPrefix(p.s[p.i:], quote) {
			p.i += len(quote)
			return
		}
		if p.s[p.i] == '\\' {
			p.i++
			if p.i >= len(p.s) {
				p.failed = true
				return
			}
			if p.s[p.i] == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n' {
				p.i += 2
				continue
			}
			p.i++
			continue
		}
		p.i++
	}
	p.failed = true
}

func (p *pyscan) scanFString(quote string) {
	triple := len(quote) == 3
	qch := quote[0]
	prev := p.triple
	p.triple = triple
	defer func() { p.triple = prev }()
	for p.i < len(p.s) && !p.failed {
		if p.s[p.i] == '\\' {
			if p.i+1 < len(p.s) && (p.s[p.i+1] == '\n' || p.s[p.i+1] == '\r') {
				p.i += 2
				if p.i <= len(p.s) && p.i > 0 && p.s[p.i-1] == '\r' && p.i < len(p.s) && p.s[p.i] == '\n' {
					p.i++
				}
				continue
			}
			if p.i+1 < len(p.s) && p.s[p.i+1] == qch {
				p.i += 2
				continue
			}
			p.i++
			continue
		}
		if !triple && (p.s[p.i] == '\n' || p.s[p.i] == '\r') {
			p.failed = true
			return
		}
		if strings.HasPrefix(p.s[p.i:], quote) {
			p.i += len(quote)
			return
		}
		if p.s[p.i] == '{' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '{' {
				p.i += 2
				continue
			}
			p.i++
			p.scanReplacement()
			continue
		}
		if p.s[p.i] == '}' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '}' {
				p.i += 2
				continue
			}
			p.failed = true
			return
		}
		p.i++
	}
	if p.i >= len(p.s) {
		p.failed = true
	}
}

func (p *pyscan) scanReplacement() {
	depth := 0
	for p.i < len(p.s) && !p.failed {
		if !p.triple && (p.s[p.i] == '\n' || p.s[p.i] == '\r') {
			// Newlines are legal inside expressions (they end comments) even in
			// single-quoted f-strings; the tokenizer accepts them.
		}
		if p.tryString() {
			continue
		}
		if p.s[p.i] == '\\' {
			if !p.consumeLineCont() {
				p.failed = true
				return
			}
			continue
		}
		if p.s[p.i] == '#' {
			p.consumeComment()
			continue
		}
		c := p.s[p.i]
		if depth == 0 && c == ':' {
			p.i++
			p.scanFormat()
			return
		}
		if depth == 0 && c == '}' {
			p.i++
			return
		}
		switch c {
		case '(', '[', '{':
			depth++
			p.i++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			p.i++
		default:
			p.i++
		}
	}
	p.failed = true
}

func (p *pyscan) scanFormat() {
	for p.i < len(p.s) && !p.failed {
		if !p.triple && (p.s[p.i] == '\n' || p.s[p.i] == '\r') {
			p.failed = true
			return
		}
		if p.s[p.i] == '{' {
			p.i++
			p.scanReplacement()
			continue
		}
		if p.s[p.i] == '}' {
			p.i++
			return
		}
		if p.s[p.i] == '\\' {
			p.i++
			if p.i < len(p.s) {
				if p.s[p.i] == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n' {
					p.i += 2
					continue
				}
				p.i++
			}
			continue
		}
		p.i++
	}
	p.failed = true
}

func skipQuoted(text string, pos int, quote byte) int {
	n := len(text)
	pos++
	for pos < n {
		ch := text[pos]
		if ch == '\\' && pos+1 < n {
			pos += 2
			continue
		}
		if ch == '\n' {
			return pos
		}
		if ch == quote {
			return pos + 1
		}
		pos++
	}
	return pos
}

func scanGoComments(text string) []string {
	var comments []string
	n := len(text)
	i := 0
	for i < n {
		ch := text[i]
		switch {
		case ch == '"' || ch == '\'':
			i = skipQuoted(text, i, ch)
		case ch == '`':
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return comments
			}
			i = i + 1 + end + 1
		case ch == '/' && i+1 < n && text[i+1] == '/':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				comments = append(comments, text[i:])
				return comments
			}
			comments = append(comments, text[i:i+end])
			i += end
		case ch == '/' && i+1 < n && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				comments = append(comments, text[i:])
				return comments
			}
			end = i + 2 + end + 2
			comments = append(comments, text[i:end])
			i = end
		default:
			i++
		}
	}
	return comments
}

func skipRustRawString(text string, pos int) (int, bool) {
	n := len(text)
	j := pos
	if j < n && (text[j] == 'b' || text[j] == 'c') {
		j++
	}
	if j >= n || text[j] != 'r' {
		return 0, false
	}
	j++
	hashes := 0
	for j < n && text[j] == '#' {
		hashes++
		j++
	}
	if j >= n || text[j] != '"' {
		return 0, false
	}
	closing := "\"" + strings.Repeat("#", hashes)
	end := strings.Index(text[j+1:], closing)
	if end < 0 {
		return n, true
	}
	return j + 1 + end + len(closing), true
}

func skipRustBlockComment(text string, pos int) int {
	n := len(text)
	depth := 1
	i := pos + 2
	for i < n && depth > 0 {
		if i+1 < n && text[i] == '/' && text[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < n && text[i] == '*' && text[i+1] == '/' {
			depth--
			i += 2
			continue
		}
		i++
	}
	return i
}

func skipRustCharLiteral(text string, pos int) (int, bool) {
	n := len(text)
	if pos+2 >= n || text[pos] != '\'' {
		return 0, false
	}
	if text[pos+1] == '\\' {
		j := pos + 2
		for j < n && text[j] != '\'' && text[j] != '\n' {
			j++
		}
		if j < n && text[j] == '\'' {
			return j + 1, true
		}
		return 0, false
	}
	if text[pos+1] != '\n' && text[pos+2] == '\'' {
		return pos + 3, true
	}
	return 0, false
}

func matchRustAttrBracket(text string, pos int) (int, bool) {
	n := len(text)
	if pos >= n || text[pos] != '#' {
		return 0, false
	}
	j := skipPySpace(text, pos+1)
	if j < n && text[j] == '!' {
		j = skipPySpace(text, j+1)
	}
	if j < n && text[j] == '[' {
		return j, true
	}
	return 0, false
}

func skipPySpace(text string, j int) int {
	for j < len(text) {
		r, sz := utf8.DecodeRuneInString(text[j:])
		if sz < 1 {
			break
		}
		if !unicode.IsSpace(r) {
			break
		}
		j += sz
	}
	return j
}

func readRustAttr(text string, pos, bracketPos int) (string, int) {
	n := len(text)
	i := bracketPos
	depth := 0
	for i < n {
		ch := text[i]
		switch ch {
		case '"':
			i = skipQuoted(text, i, '"')
		case '[':
			depth++
			i++
		case ']':
			depth--
			i++
			if depth == 0 {
				return text[pos:i], i
			}
		default:
			i++
		}
	}
	return text[pos:i], i
}

func skipRustNonAttr(text string, pos int) (int, bool) {
	n := len(text)
	ch := text[pos]
	if ch == '/' && pos+1 < n && text[pos+1] == '/' {
		end := strings.IndexByte(text[pos:], '\n')
		if end < 0 {
			return n, true
		}
		return pos + end, true
	}
	if ch == '/' && pos+1 < n && text[pos+1] == '*' {
		return skipRustBlockComment(text, pos), true
	}
	if ch == '"' {
		return skipQuoted(text, pos, '"'), true
	}
	if ch == 'r' || ch == 'b' || ch == 'c' {
		if end, ok := skipRustRawString(text, pos); ok {
			return end, true
		}
		if (ch == 'b' || ch == 'c') && pos+1 < n && text[pos+1] == '"' {
			return skipQuoted(text, pos+1, '"'), true
		}
	}
	if ch == '\'' {
		if end, ok := skipRustCharLiteral(text, pos); ok {
			return end, true
		}
		return pos + 1, true
	}
	return 0, false
}

func countRustAllows(text string) int {
	count := 0
	n := len(text)
	i := 0
	for i < n {
		if skipped, ok := skipRustNonAttr(text, i); ok {
			i = skipped
			continue
		}
		if text[i] == '#' {
			if bracket, ok := matchRustAttrBracket(text, i); ok {
				attr, next := readRustAttr(text, i, bracket)
				if rustAllow.MatchString(attr) {
					count++
				}
				i = next
				continue
			}
		}
		i++
	}
	return count
}

func skipTSTemplate(text string, pos int) int {
	n := len(text)
	i := pos + 1
	for i < n {
		ch := text[i]
		if ch == '\\' && i+1 < n {
			i += 2
			continue
		}
		if ch == '`' {
			return i + 1
		}
		i++
	}
	return i
}

func scanTSComments(text string, svelte bool) []string {
	var comments []string
	n := len(text)
	i := 0
	for i < n {
		ch := text[i]
		switch {
		case ch == '"' || ch == '\'':
			i = skipQuoted(text, i, ch)
		case ch == '`':
			i = skipTSTemplate(text, i)
		case ch == '/' && i+1 < n && text[i+1] == '/':
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				comments = append(comments, text[i:])
				return comments
			}
			comments = append(comments, text[i:i+end])
			i += end
		case ch == '/' && i+1 < n && text[i+1] == '*':
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				comments = append(comments, text[i:])
				return comments
			}
			end = i + 2 + end + 2
			comments = append(comments, text[i:end])
			i = end
		case svelte && i+3 < n && text[i:i+4] == "<!--":
			end := strings.Index(text[i+4:], "-->")
			if end < 0 {
				comments = append(comments, text[i:])
				return comments
			}
			end = i + 4 + end + 3
			comments = append(comments, text[i:end])
			i = end
		default:
			i++
		}
	}
	return comments
}

func resolveTarget(rel string, targets []string) string {
	norm := trimLeftSet(rel, "./")
	best := ""
	for _, target := range targets {
		prefix := target + "/"
		if strings.HasPrefix(norm, prefix) && len(target) > len(best) {
			best = target
		}
	}
	if best == "" {
		return "repo-root"
	}
	return best
}
