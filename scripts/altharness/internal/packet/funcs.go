package packet

import (
	"fmt"
	"regexp"
	"strings"
)

func normalizeGoReceiver(raw string) string {
	tokens := strings.Fields(strings.TrimSpace(raw))
	if len(tokens) == 0 {
		return ""
	}
	typeTok := tokens[len(tokens)-1]
	for strings.HasPrefix(typeTok, "(") && strings.HasSuffix(typeTok, ")") && len(typeTok) >= 2 {
		typeTok = strings.TrimSpace(typeTok[1 : len(typeTok)-1])
	}
	typeTok = strings.TrimSpace(strings.TrimLeft(typeTok, "*"))
	for strings.HasPrefix(typeTok, "(") && strings.HasSuffix(typeTok, ")") && len(typeTok) >= 2 {
		typeTok = strings.TrimSpace(typeTok[1 : len(typeTok)-1])
	}
	return typeTok
}

func extractGoFuncBodies(source string) map[string]string {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}
	}
	masked, keep := maskCommentsAndStrings(source, "go")
	funcs := map[string]string{}
	counts := map[string]int{}
	pos := 0
	for pos < len(masked) {
		loc := goFuncDeclRe.FindStringSubmatchIndex(masked[pos:])
		if loc == nil {
			break
		}
		absStart := pos + loc[0]
		absEnd := pos + loc[1]
		prefix := masked[:absStart]
		if strings.Count(prefix, "{")-strings.Count(prefix, "}") != 0 {
			pos = absEnd
			continue
		}
		bodyOpen, ok := findBodyBraceAfterParams(masked, absEnd-1, "go")
		if !ok {
			pos = absEnd
			continue
		}
		bodyClose := findMaskedBraceClose(masked, bodyOpen)
		recv := ""
		if loc[2] >= 0 && loc[3] >= loc[2] {
			recv = masked[pos+loc[2] : pos+loc[3]]
		}
		name := ""
		if loc[4] >= 0 && loc[5] >= loc[4] {
			name = masked[pos+loc[4] : pos+loc[5]]
		}
		recvType := normalizeGoReceiver(recv)
		base := name
		if recvType != "" {
			base = "(" + recvType + ")." + name
		}
		counts[base]++
		key := base
		if counts[base] > 1 {
			key = fmt.Sprintf("%s#%d", base, counts[base])
		}
		end := bodyClose
		if end > len(keep) {
			end = len(keep)
		}
		start := absStart
		if start < 0 {
			start = 0
		}
		if start > end {
			start = end
		}
		funcs[key] = hashNormalized(keep[start:end])
		if bodyClose > absEnd {
			pos = bodyClose
		} else {
			pos = absEnd
		}
	}
	return funcs
}

func extractRustImplRanges(masked string) [][3]int {
	return nil
}

type rustImpl struct {
	open, close int
	target      string
}

func rustImplRanges(masked string) []rustImpl {
	var ranges []rustImpl
	for _, loc := range rustImplDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		open := loc[1] - 1
		close := findMaskedBraceClose(masked, open)
		target := ""
		if loc[2] >= 0 && loc[3] >= loc[2] {
			target = strings.Join(strings.Fields(masked[loc[2]:loc[3]]), " ")
		}
		if i := strings.Index(target, " where "); i >= 0 {
			target = strings.TrimSpace(target[:i])
		}
		ranges = append(ranges, rustImpl{open: open, close: close, target: target})
	}
	return ranges
}

func rustFnDeclStart(masked string, fnStart int) int {
	lineStart := strings.LastIndex(masked[:fnStart], "\n") + 1
	prefix := masked[lineStart:fnStart]
	loc := rustPreFnRe.FindStringIndex(prefix)
	if loc == nil {
		return fnStart
	}
	return lineStart + loc[0]
}

func extractRustFuncBodies(source string) map[string]string {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}
	}
	masked, keep := maskCommentsAndStrings(source, "rust")
	ranges := rustImplRanges(masked)
	funcs := map[string]string{}
	counts := map[string]int{}
	for _, loc := range rustFnDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		bodyOpen, ok := findRustFnBodyOpen(masked, loc[1])
		if !ok {
			continue
		}
		bodyClose := findMaskedBraceClose(masked, bodyOpen)
		name := masked[loc[2]:loc[3]]
		var enclosing []string
		for _, r := range ranges {
			if r.open < loc[0] && loc[0] < r.close {
				enclosing = append(enclosing, r.target)
			}
		}
		base := name
		if len(enclosing) > 0 {
			base = enclosing[len(enclosing)-1] + "::" + name
		}
		counts[base]++
		key := base
		if counts[base] > 1 {
			key = fmt.Sprintf("%s#%d", base, counts[base])
		}
		declStart := rustFnDeclStart(masked, loc[0])
		end := bodyClose
		if end > len(keep) {
			end = len(keep)
		}
		if declStart < 0 {
			declStart = 0
		}
		if declStart > end {
			declStart = end
		}
		funcs[key] = hashNormalized(keep[declStart:end])
	}
	return funcs
}

func extractSvelteScripts(source string) string {
	matches := svelteScriptRe.FindAllStringSubmatch(source, -1)
	var blocks []string
	for _, m := range matches {
		if len(m) > 1 {
			blocks = append(blocks, m[1])
		}
	}
	return strings.Join(blocks, "\n")
}

func recordTSFunc(funcs map[string]string, counts map[string]int, base, span string) {
	counts[base]++
	key := base
	if counts[base] > 1 {
		key = fmt.Sprintf("%s#%d", base, counts[base])
	}
	funcs[key] = hashNormalized(span)
}

func extractTSFuncBodies(source string, svelte bool) map[string]string {
	effective := source
	if svelte {
		effective = extractSvelteScripts(source)
	}
	if strings.TrimSpace(effective) == "" {
		return map[string]string{}
	}
	masked, keep := maskCommentsAndStrings(effective, "typescript")
	funcs := map[string]string{}
	counts := map[string]int{}
	seen := map[[2]int]struct{}{}
	type classRange struct {
		open, close int
		name        string
	}
	var classes []classRange
	for _, loc := range tsClassDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		open := loc[1] - 1
		close := findMaskedBraceClose(masked, open)
		name := ""
		if loc[2] >= 0 {
			name = masked[loc[2]:loc[3]]
		}
		classes = append(classes, classRange{open, close, name})
	}
	recordSpan := func(base string, start, end int, open, close int, mark bool) {
		if start < 0 {
			start = 0
		}
		if end > len(keep) {
			end = len(keep)
		}
		if start > end {
			start = end
		}
		recordTSFunc(funcs, counts, base, keep[start:end])
		if mark {
			seen[[2]int{open, close}] = struct{}{}
		}
	}
	for _, loc := range tsFuncDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		bodyOpen, ok := findBodyBraceAfterParams(masked, loc[1]-1, "typescript")
		if !ok {
			continue
		}
		bodyClose := findMaskedBraceClose(masked, bodyOpen)
		name := masked[loc[2]:loc[3]]
		recordSpan(name, loc[0], bodyClose, bodyOpen, bodyClose, true)
	}
	for _, loc := range tsArrowDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		tail := loc[1]
		for tail < len(masked) && isASCIISpace(masked[tail]) {
			tail++
		}
		bodyClose := len(masked)
		mark := false
		open := tail
		if tail < len(masked) && masked[tail] == '{' {
			bodyClose = findMaskedBraceClose(masked, tail)
			mark = true
			open = tail
		} else {
			semi := strings.IndexByte(masked[tail:], ';')
			nl := strings.IndexByte(masked[tail:], '\n')
			ends := []int{}
			if semi >= 0 {
				ends = append(ends, tail+semi)
			}
			if nl >= 0 {
				ends = append(ends, tail+nl)
			}
			if len(ends) > 0 {
				bodyClose = ends[0]
				if len(ends) > 1 && ends[1] < bodyClose {
					bodyClose = ends[1]
				}
			}
		}
		name := masked[loc[2]:loc[3]]
		recordSpan(name, loc[0], bodyClose, open, bodyClose, mark)
	}
	for _, loc := range tsMethodDeclRe.FindAllStringSubmatchIndex(masked, -1) {
		name := masked[loc[2]:loc[3]]
		if _, skip := tsControlKeywords[name]; skip {
			continue
		}
		bodyOpen := loc[1] - 1
		bodyClose := findMaskedBraceClose(masked, bodyOpen)
		if _, ok := seen[[2]int{bodyOpen, bodyClose}]; ok {
			continue
		}
		var enclosing []string
		for _, c := range classes {
			if c.open < loc[0] && loc[0] < c.close {
				enclosing = append(enclosing, c.name)
			}
		}
		base := name
		if len(enclosing) > 0 {
			base = enclosing[len(enclosing)-1] + "." + name
		}
		recordSpan(base, loc[0], bodyClose, bodyOpen, bodyClose, true)
	}
	return funcs
}

func countModifiedFunctionsInFile(rel, before, after string) (int, error) {
	lang := inferLanguage(rel)
	var bMap, aMap map[string]string
	var err error
	switch lang {
	case "python":
		bMap, err = extractPythonFuncBodies(before)
		if err != nil {
			return 0, pErr("failed to parse Python file %s: %s", rel, err.Error())
		}
		aMap, err = extractPythonFuncBodies(after)
		if err != nil {
			return 0, pErr("failed to parse Python file %s: %s", rel, err.Error())
		}
	case "go":
		bMap = extractGoFuncBodies(before)
		aMap = extractGoFuncBodies(after)
	case "rust":
		bMap = extractRustFuncBodies(before)
		aMap = extractRustFuncBodies(after)
	case "typescript":
		svelte := strings.HasSuffix(strings.ToLower(rel), ".svelte")
		bMap = extractTSFuncBodies(before, svelte)
		aMap = extractTSFuncBodies(after, svelte)
	default:
		return 0, nil
	}
	changed := map[string]struct{}{}
	for k := range bMap {
		if _, ok := aMap[k]; !ok {
			changed[k] = struct{}{}
		}
	}
	for k := range aMap {
		if _, ok := bMap[k]; !ok {
			changed[k] = struct{}{}
		}
	}
	for k := range bMap {
		if a, ok := aMap[k]; ok && bMap[k] != a {
			changed[k] = struct{}{}
		}
	}
	return len(changed), nil
}

func countActualModifiedFunctions(ctxRel func(rel string) (string, string, error), files []string) (int, error) {
	total := 0
	for _, rel := range files {
		before, after, err := ctxRel(rel)
		if err != nil {
			return 0, err
		}
		if before == after {
			continue
		}
		n, err := countModifiedFunctionsInFile(rel, before, after)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

var (
	pyFuncHeadRe  = regexp.MustCompile(`^(?:(async)\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pyClassHeadRe = regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
)

type logicalLine struct {
	indent int
	text   string
}

func countIndent(line string) int {
	i := 0
	for i < len(line) {
		switch line[i] {
		case ' ':
			i++
		case '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func firstContent(line logicalLine) string {
	raw := line.text
	if i := strings.IndexByte(raw, '\n'); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}

func isPyDecorator(line logicalLine) bool {
	return strings.HasPrefix(firstContent(line), "@")
}

func pyHeader(line logicalLine) (kind, name string, ok bool) {
	s := firstContent(line)
	if m := pyFuncHeadRe.FindStringSubmatch(s); m != nil {
		kind = "FunctionDef"
		if m[1] == "async" {
			kind = "AsyncFunctionDef"
		}
		return kind, m[2], true
	}
	if m := pyClassHeadRe.FindStringSubmatch(s); m != nil {
		return "class", m[1], true
	}
	return "", "", false
}

func extractPythonFuncBodies(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	return extractPythonFuncBodiesPureGo(source)
}

func pythonLogicalLinesStrict(src string) ([]logicalLine, error) {
	phys := splitLines(src)
	var out []logicalLine
	var buf strings.Builder
	baseIndent := 0
	started := false
	var stack []byte
	var quote byte
	triple := false
	escaped := false
	for lineNo, line := range phys {
		if !started && quote == 0 && len(stack) == 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
		}
		if !started {
			baseIndent = countIndent(line)
			started = true
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
		var scanErr error
		stack, quote, triple, escaped, scanErr = scanPyLineStateStrict(line, lineNo+1, stack, quote, triple, escaped)
		if scanErr != nil {
			return nil, scanErr
		}
		if quote != 0 && !triple && !strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) {
			return nil, pErr("unterminated string literal at line %d", lineNo+1)
		}
		cont := len(stack) > 0 || quote != 0 || strings.HasSuffix(strings.TrimRight(line, " \t"), `\`)
		if cont {
			continue
		}
		out = append(out, logicalLine{indent: baseIndent, text: strings.TrimRight(buf.String(), "\n")})
		buf.Reset()
		started = false
		escaped = false
	}
	if quote != 0 {
		return nil, pErr("unterminated string literal at EOF")
	}
	if len(stack) != 0 {
		return nil, pErr("unclosed bracket/parenthesis %q at EOF", string(stack[len(stack)-1]))
	}
	if buf.Len() > 0 {
		out = append(out, logicalLine{indent: baseIndent, text: strings.TrimRight(buf.String(), "\n")})
	}
	return out, nil
}

func matchingCloseBracket(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	default:
		return 0
	}
}

func scanPyLineStateStrict(line string, lineNo int, stack []byte, quote byte, triple, escaped bool) ([]byte, byte, bool, bool, error) {
	i := 0
	for i < len(line) {
		ch := line[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if quote != 0 {
			if ch == '\\' {
				escaped = true
				i++
				continue
			}
			if triple && i+2 < len(line) && line[i] == quote && line[i+1] == quote && line[i+2] == quote {
				quote = 0
				triple = false
				i += 3
				continue
			}
			if !triple && ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '#' {
			break
		}
		if ch == '\\' {
			escaped = true
			i++
			continue
		}
		if (ch == '"' || ch == '\'') && i+2 < len(line) && line[i+1] == ch && line[i+2] == ch {
			quote = ch
			triple = true
			i += 3
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			triple = false
			i++
			continue
		}
		switch ch {
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 {
				return nil, quote, triple, escaped, pErr("unmatched closing bracket %q at line %d", string(ch), lineNo)
			}
			open := stack[len(stack)-1]
			if matchingCloseBracket(open) != ch {
				return nil, quote, triple, escaped, pErr("mismatched bracket %q closed by %q at line %d", string(open), string(ch), lineNo)
			}
			stack = stack[:len(stack)-1]
		}
		i++
	}
	return stack, quote, triple, escaped, nil
}

type pyTokKind int

const (
	pyTokIdent pyTokKind = iota
	pyTokNumber
	pyTokString
	pyTokOp
)

type pyToken struct {
	kind   pyTokKind
	text   string
	prefix string
}

var pyNonCallKeywords = map[string]struct{}{
	"return": {}, "yield": {}, "assert": {}, "raise": {},
	"if": {}, "elif": {}, "while": {}, "for": {}, "in": {},
	"not": {}, "and": {}, "or": {}, "is": {}, "del": {},
	"await": {}, "lambda": {}, "as": {}, "from": {}, "import": {},
	"except": {}, "with": {}, "case": {}, "match": {},
}

var pyAssignOps = map[string]struct{}{
	"=": {}, "+=": {}, "-=": {}, "*=": {}, "/=": {}, "//=": {},
	"%=": {}, "**=": {}, "&=": {}, "|=": {}, "^=": {}, ">>=": {},
	"<<=": {}, "@=": {},
}

var pyPrefixExprKeywords = map[string]struct{}{
	"return": {}, "yield": {}, "raise": {}, "assert": {}, "del": {}, "await": {},
}

var pyCondKeywords = map[string]struct{}{
	"if": {}, "elif": {}, "while": {},
}

var pyCompoundHeadRe = regexp.MustCompile(`^(?:(?:async\s+)?(?:for|with)|if|elif|else|while|try|except|finally)\b`)

func normalizePyPrefix(raw string) string {
	low := strings.ToLower(raw)
	hasR, hasB, hasF := false, false, false
	for i := 0; i < len(low); i++ {
		switch low[i] {
		case 'r':
			hasR = true
		case 'b':
			hasB = true
		case 'f':
			hasF = true
		}
	}
	var b strings.Builder
	if hasF {
		b.WriteByte('f')
	}
	if hasB {
		b.WriteByte('b')
	}
	if hasR {
		b.WriteByte('r')
	}
	return b.String()
}

func normalizePyStringToken(prefix, seg string) string {
	normPrefix := normalizePyPrefix(prefix)
	if len(seg) < 2 {
		return normPrefix + seg
	}
	q := seg[0]
	if q != '\'' && q != '"' {
		return normPrefix + seg
	}
	isTriple := len(seg) >= 6 && seg[1] == q && seg[2] == q && seg[len(seg)-1] == q && seg[len(seg)-2] == q && seg[len(seg)-3] == q
	if isTriple {
		if q == '\'' && !strings.Contains(normPrefix, "r") && !strings.Contains(normPrefix, "f") {
			inner := seg[3 : len(seg)-3]
			if !strings.Contains(inner, `"""`) && !strings.HasSuffix(inner, `"`) {
				return normPrefix + `"""` + inner + `"""`
			}
		}
		return normPrefix + seg
	}
	if strings.Contains(normPrefix, "r") {
		if q == '\'' {
			inner := seg[1 : len(seg)-1]
			if !strings.Contains(inner, `"`) && !strings.Contains(inner, `\'`) {
				return normPrefix + `"` + inner + `"`
			}
		}
		return normPrefix + seg
	}
	if strings.Contains(normPrefix, "f") {
		if q == '\'' {
			inner := seg[1 : len(seg)-1]
			if !strings.Contains(inner, `"`) && !strings.Contains(inner, `\'`) {
				return normPrefix + `"` + inner + `"`
			}
		}
		return normPrefix + seg
	}
	return normPrefix + normalizePyQuotedString(seg)
}

func isPyStringPrefixChars(s string) bool {
	if len(s) == 0 || len(s) > 2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'r', 'R', 'b', 'B', 'u', 'U', 'f', 'F':
		default:
			return false
		}
	}
	return true
}

func tokenizePy(text string) []pyToken {
	var toks []pyToken
	n := len(text)
	i := 0
	for i < n {
		ch := text[i]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			i++
			continue
		}
		if ch == '\\' {
			i++
			if i < n && text[i] == '\r' {
				i++
			}
			if i < n && text[i] == '\n' {
				i++
			}
			continue
		}
		if ch == '#' {
			for i < n && text[i] != '\n' {
				i++
			}
			continue
		}
		// Check prefixed string literal (e.g. r"...", f'...', rb"...")
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_' || ch >= 0x80 {
			start := i
			for i < n && isIdentByte(text[i]) {
				i++
			}
			word := text[start:i]
			if i < n && (text[i] == '"' || text[i] == '\'') && isPyStringPrefixChars(word) {
				q := text[i]
				segStart := i
				end := scanPyStringEnd(text, i, q)
				i = end
				normPrefix := normalizePyPrefix(word)
				toks = append(toks, pyToken{
					kind:   pyTokString,
					text:   normalizePyStringToken(word, text[segStart:end]),
					prefix: normPrefix,
				})
				continue
			}
			toks = append(toks, pyToken{kind: pyTokIdent, text: word})
			continue
		}
		if ch == '"' || ch == '\'' {
			end := scanPyStringEnd(text, i, ch)
			toks = append(toks, pyToken{
				kind:   pyTokString,
				text:   normalizePyStringToken("", text[i:end]),
				prefix: "",
			})
			i = end
			continue
		}
		if ch >= '0' && ch <= '9' {
			start := i
			for i < n && (isIdentByte(text[i]) || text[i] == '.') {
				i++
			}
			toks = append(toks, pyToken{kind: pyTokNumber, text: text[start:i]})
			continue
		}
		// Multi-character operators
		if i+2 < n {
			tri := text[i : i+3]
			switch tri {
			case "...", "//=", "**=", ">>=", "<<=":
				toks = append(toks, pyToken{kind: pyTokOp, text: tri})
				i += 3
				continue
			}
		}
		if i+1 < n {
			duo := text[i : i+2]
			switch duo {
			case "==", "!=", "<=", ">=", "**", "//", "<<", ">>", "->", ":=",
				"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "@=":
				toks = append(toks, pyToken{kind: pyTokOp, text: duo})
				i += 2
				continue
			}
		}
		toks = append(toks, pyToken{kind: pyTokOp, text: text[i : i+1]})
		i++
	}
	return mergeAdjacentPyStrings(toks)
}

func scanPyStringEnd(text string, start int, q byte) int {
	n := len(text)
	if start+2 < n && text[start+1] == q && text[start+2] == q {
		i := start + 3
		for i < n {
			if text[i] == '\\' && i+1 < n {
				i += 2
				continue
			}
			if i+2 < n && text[i] == q && text[i+1] == q && text[i+2] == q {
				return i + 3
			}
			i++
		}
		return n
	}
	i := start + 1
	for i < n {
		if text[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		if text[i] == q {
			return i + 1
		}
		i++
	}
	return n
}

func mergeAdjacentPyStrings(toks []pyToken) []pyToken {
	if len(toks) < 2 {
		return toks
	}
	out := make([]pyToken, 0, len(toks))
	for _, tok := range toks {
		if len(out) > 0 && tok.kind == pyTokString && out[len(out)-1].kind == pyTokString &&
			tok.prefix == out[len(out)-1].prefix && !strings.Contains(tok.prefix, "r") && !strings.Contains(tok.prefix, "f") {
			prev := out[len(out)-1]
			if isSimpleDoubleQuoted(prev.text, prev.prefix) && isSimpleDoubleQuoted(tok.text, tok.prefix) {
				pBody := prev.text[len(prev.prefix)+1 : len(prev.text)-1]
				tBody := tok.text[len(tok.prefix)+1 : len(tok.text)-1]
				out[len(out)-1].text = prev.prefix + `"` + pBody + tBody + `"`
				continue
			}
		}
		out = append(out, tok)
	}
	return out
}

func isSimpleDoubleQuoted(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix+`"`) || !strings.HasSuffix(s, `"`) {
		return false
	}
	body := s[len(prefix):]
	if len(body) < 2 {
		return false
	}
	if len(body) >= 6 && strings.HasPrefix(body, `"""`) && strings.HasSuffix(body, `"""`) {
		return false
	}
	return true
}

func normalizePyQuotedString(seg string) string {
	if len(seg) < 2 {
		return seg
	}
	q := seg[0]
	if (q != '\'' && q != '"') || seg[len(seg)-1] != q {
		return seg
	}
	if len(seg) >= 6 && seg[1] == q && seg[2] == q && seg[len(seg)-2] == q && seg[len(seg)-3] == q {
		return seg
	}
	inner := seg[1 : len(seg)-1]
	var out strings.Builder
	out.WriteByte('"')
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch == '\\' && i+1 < len(inner) {
			nxt := inner[i+1]
			if q == '\'' && nxt == '\'' {
				out.WriteByte('\'')
				i++
				continue
			}
			if q == '"' && nxt == '"' {
				out.WriteString(`\"`)
				i++
				continue
			}
			out.WriteByte(ch)
			out.WriteByte(nxt)
			i++
			continue
		}
		if ch == '"' {
			out.WriteString(`\"`)
			continue
		}
		out.WriteByte(ch)
	}
	out.WriteByte('"')
	return out.String()
}

func matchingPyClose(toks []pyToken, openIdx int) int {
	open := toks[openIdx].text
	closeTok := ")"
	switch open {
	case "[":
		closeTok = "]"
	case "{":
		closeTok = "}"
	}
	depth := 0
	for i := openIdx; i < len(toks); i++ {
		switch toks[i].text {
		case open:
			depth++
		case closeTok:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func stripTrailingPyCommas(toks []pyToken) []pyToken {
	type frame struct {
		open       string
		isCallLike bool
		commaCount int
	}
	var stack []frame
	drop := make([]bool, len(toks))
	for i, tok := range toks {
		switch tok.text {
		case "(":
			callLike := false
			if i > 0 {
				prev := toks[i-1]
				if prev.kind == pyTokIdent {
					if _, kw := pyNonCallKeywords[prev.text]; !kw {
						callLike = true
					}
				} else if prev.text == ")" || prev.text == "]" || prev.text == "}" {
					callLike = true
				}
			}
			stack = append(stack, frame{open: "(", isCallLike: callLike})
		case "[", "{":
			stack = append(stack, frame{open: tok.text, isCallLike: true})
		case ",":
			if len(stack) > 0 {
				stack[len(stack)-1].commaCount++
			}
		case ")", "]", "}":
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if i > 0 && toks[i-1].text == "," {
					if tok.text != ")" || top.isCallLike || top.commaCount > 1 {
						drop[i-1] = true
					}
				}
			}
		}
	}
	out := make([]pyToken, 0, len(toks))
	for i, tok := range toks {
		if !drop[i] {
			out = append(out, tok)
		}
	}
	return out
}

func stripOuterGroupingParens(toks []pyToken) []pyToken {
	for len(toks) >= 2 && toks[0].text == "(" && matchingPyClose(toks, 0) == len(toks)-1 {
		inner := toks[1 : len(toks)-1]
		if len(inner) == 0 {
			break
		}
		depth := 0
		keep := false
		for _, tok := range inner {
			switch tok.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			default:
				if depth == 0 && (tok.text == "," || tok.text == ":=" ||
					(tok.kind == pyTokIdent && (tok.text == "for" || tok.text == "async" || tok.text == "yield"))) {
					keep = true
				}
			}
			if keep {
				break
			}
		}
		if keep {
			break
		}
		toks = inner
	}
	return toks
}

func normalizePyStmtTokens(toks []pyToken) []pyToken {
	if len(toks) == 0 {
		return toks
	}
	toks = stripTrailingPyCommas(toks)
	toks = mergeAdjacentPyStrings(toks)
	if len(toks) == 0 {
		return toks
	}
	// Leading expression keyword: return (expr), yield (expr), assert (expr), etc.
	if toks[0].kind == pyTokIdent {
		if _, ok := pyPrefixExprKeywords[toks[0].text]; ok && len(toks) > 1 {
			rhs := stripOuterGroupingParens(toks[1:])
			out := make([]pyToken, 0, 1+len(rhs))
			out = append(out, toks[0])
			out = append(out, rhs...)
			return out
		}
		if _, ok := pyCondKeywords[toks[0].text]; ok && len(toks) > 2 && toks[len(toks)-1].text == ":" {
			cond := stripOuterGroupingParens(toks[1 : len(toks)-1])
			out := make([]pyToken, 0, 2+len(cond))
			out = append(out, toks[0])
			out = append(out, cond...)
			out = append(out, toks[len(toks)-1])
			return out
		}
	}
	// Top-level assignment: lhs = (expr), lhs: T = (expr), lhs += (expr)
	depth := 0
	for i, tok := range toks {
		switch tok.text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		default:
			if depth == 0 && tok.kind == pyTokOp {
				if _, ok := pyAssignOps[tok.text]; ok && i+1 < len(toks) {
					rhs := stripOuterGroupingParens(toks[i+1:])
					out := make([]pyToken, 0, i+1+len(rhs))
					out = append(out, toks[:i+1]...)
					out = append(out, rhs...)
					return out
				}
			}
		}
	}
	return stripOuterGroupingParens(toks)
}

func collapseCode(text string) string {
	toks := normalizePyStmtTokens(tokenizePy(text))
	if len(toks) == 0 {
		return ""
	}
	parts := make([]string, len(toks))
	for i, tok := range toks {
		parts[i] = tok.text
	}
	return strings.Join(parts, " ")
}

func findTopLevelColon(text string) int {
	depth := 0
	var quote byte
	triple := false
	escaped := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if ch == '\\' {
				escaped = true
				continue
			}
			if triple && i+2 < len(text) && text[i] == quote && text[i+1] == quote && text[i+2] == quote {
				quote = 0
				triple = false
				i += 2
				continue
			}
			if !triple && ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '#' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if (ch == '"' || ch == '\'') && i+2 < len(text) && text[i+1] == ch && text[i+2] == ch {
			quote = ch
			triple = true
			i += 2
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			triple = false
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

var (
	pyDefLikeRe   = regexp.MustCompile(`^(?:async\s+)?def\b`)
	pyClassLikeRe = regexp.MustCompile(`^class\b`)
)

func extractPythonFuncBodiesPureGo(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	rawLines, err := pythonLogicalLinesStrict(source)
	if err != nil {
		return nil, err
	}
	lines := make([]logicalLine, 0, len(rawLines))
	for _, line := range rawLines {
		head := firstContent(line)
		switch {
		case pyDefLikeRe.MatchString(head):
			if _, _, ok := pyHeader(line); !ok {
				return nil, pErr("invalid def header: %q", head)
			}
			colon := findTopLevelColon(line.text)
			if colon < 0 {
				return nil, pErr("def header missing ':': %q", head)
			}
			tail := strings.TrimSpace(collapseCode(line.text[colon+1:]))
			if tail != "" {
				lines = append(lines,
					logicalLine{indent: line.indent, text: line.text[:colon+1]},
					logicalLine{indent: line.indent + 4, text: line.text[colon+1:]},
				)
				continue
			}
		case pyClassLikeRe.MatchString(head):
			if _, _, ok := pyHeader(line); !ok {
				return nil, pErr("invalid class header: %q", head)
			}
			colon := findTopLevelColon(line.text)
			if colon < 0 {
				return nil, pErr("class header missing ':': %q", head)
			}
			tail := strings.TrimSpace(collapseCode(line.text[colon+1:]))
			if tail != "" {
				lines = append(lines,
					logicalLine{indent: line.indent, text: line.text[:colon+1]},
					logicalLine{indent: line.indent + 4, text: line.text[colon+1:]},
				)
				continue
			}
		case pyCompoundHeadRe.MatchString(head):
			if findTopLevelColon(line.text) < 0 {
				return nil, pErr("compound statement missing ':': %q", head)
			}
		}
		lines = append(lines, line)
	}
	funcs := map[string]string{}
	counts := map[string]int{}
	var walkErr error
	var walk func(start, end, minIndent int, prefix string) int
	walk = func(start, end, minIndent int, prefix string) int {
		i := start
		for i < end && walkErr == nil {
			if strings.TrimSpace(lines[i].text) == "" {
				i++
				continue
			}
			if lines[i].indent < minIndent {
				return i
			}
			dec := i
			for dec < end && isPyDecorator(lines[dec]) && lines[dec].indent >= minIndent {
				dec++
			}
			if dec >= end || lines[dec].indent < minIndent {
				i++
				continue
			}
			kind, name, ok := pyHeader(lines[dec])
			if !ok {
				i++
				continue
			}
			headerIndent := lines[dec].indent
			bodyStart := dec + 1
			j := bodyStart
			for j < end {
				if strings.TrimSpace(lines[j].text) == "" {
					j++
					continue
				}
				if lines[j].indent <= headerIndent {
					break
				}
				j++
			}
			if j == bodyStart {
				walkErr = pErr("empty body for %s %q", kind, name)
				return end
			}
			if kind == "class" {
				cls := name
				if prefix != "" {
					cls = prefix + "." + name
				}
				walk(bodyStart, j, headerIndent+1, cls)
				i = j
				continue
			}
			base := name
			if prefix != "" {
				base = prefix + "." + name
			}
			counts[base]++
			key := base
			if counts[base] > 1 {
				key = fmt.Sprintf("%s#%d", base, counts[base])
			}
			funcs[key] = fingerprintPython(kind, name, lines[i:dec+1], lines[bodyStart:j], headerIndent)
			walk(bodyStart, j, headerIndent+1, base)
			i = j
		}
		return end
	}
	walk(0, len(lines), 0, "")
	if walkErr != nil {
		return nil, walkErr
	}
	return funcs, nil
}

func fingerprintPython(kind, name string, header, body []logicalLine, headerIndent int) string {
	var b strings.Builder
	b.WriteString(kind)
	b.WriteByte('\n')
	b.WriteString(name)
	b.WriteByte('\n')
	for _, h := range header {
		if norm := collapseCode(h.text); norm != "" {
			b.WriteString(norm)
			b.WriteByte('\n')
		}
	}
	skipAt := -1
	for idx := 0; idx < len(body); idx++ {
		line := body[idx]
		if strings.TrimSpace(line.text) == "" {
			continue
		}
		if skipAt >= 0 {
			if line.indent > skipAt {
				continue
			}
			skipAt = -1
		}
		if line.indent > headerIndent && isPyDecorator(line) {
			k := idx
			for k < len(body) && (strings.TrimSpace(body[k].text) == "" || (isPyDecorator(body[k]) && body[k].indent == line.indent)) {
				k++
			}
			if k < len(body) && body[k].indent == line.indent {
				if _, _, ok := pyHeader(body[k]); ok {
					skipAt = body[k].indent
					idx = k
					continue
				}
			}
		}
		if line.indent > headerIndent {
			if _, _, ok := pyHeader(line); ok {
				skipAt = line.indent
				continue
			}
		}
		if norm := collapseCode(line.text); norm != "" {
			b.WriteString(norm)
			b.WriteByte('\n')
		}
	}
	return hashNormalized(b.String())
}
