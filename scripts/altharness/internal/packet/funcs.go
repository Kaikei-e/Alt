package packet

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
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

const pyFuncASTScript = `import ast, collections, hashlib, json, sys

source = sys.stdin.read()
if not source.strip():
    json.dump({}, sys.stdout)
    raise SystemExit(0)
try:
    tree = ast.parse(source)
except SyntaxError as exc:
    sys.stderr.write(f"{exc}\n")
    raise SystemExit(1)

def fp(fn):
    filtered_body = [
        stmt for stmt in fn.body
        if not isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
    ]
    parts = [
        type(fn).__name__,
        fn.name,
        ast.dump(fn.args, include_attributes=False),
        ast.dump(fn.returns, include_attributes=False) if fn.returns is not None else "",
        *(ast.dump(dec, include_attributes=False) for dec in fn.decorator_list),
        *(ast.dump(stmt, include_attributes=False) for stmt in filtered_body),
    ]
    return hashlib.sha256("\n".join(parts).encode("utf-8")).hexdigest()

funcs = {}
counts = collections.Counter()

def walk(stmts, prefix):
    for stmt in stmts:
        if isinstance(stmt, ast.ClassDef):
            cls_prefix = f"{prefix}.{stmt.name}" if prefix else stmt.name
            walk(stmt.body, cls_prefix)
        elif isinstance(stmt, (ast.FunctionDef, ast.AsyncFunctionDef)):
            base = f"{prefix}.{stmt.name}" if prefix else stmt.name
            counts[base] += 1
            key = f"{base}#{counts[base]}" if counts[base] > 1 else base
            funcs[key] = fp(stmt)
            walk(stmt.body, base)

walk(tree.body, "")
json.dump(funcs, sys.stdout)
`

func extractPythonFuncBodies(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	if py, err := exec.LookPath("python3"); err == nil {
		return extractPythonFuncBodiesAST(py, source)
	}
	return extractPythonFuncBodiesPureGo(source)
}

func extractPythonFuncBodiesAST(py, source string) (map[string]string, error) {
	res := runProcessWithStdin(context.Background(), []string{py, "-c", pyFuncASTScript}, "", []byte(source), subprocessTimeoutSeconds*time.Second)
	if res.Timeout {
		return nil, pErr("python3 AST parser timed out after %ds", subprocessTimeoutSeconds)
	}
	if res.Err != nil {
		return nil, res.Err
	}
	if res.Code != 0 {
		msg := strings.TrimSpace(validUTF8(res.Stderr))
		if msg == "" {
			msg = fmt.Sprintf("python3 AST parser exited with code %d", res.Code)
		}
		return nil, pErr("%s", msg)
	}
	var funcs map[string]string
	if err := json.Unmarshal(res.Stdout, &funcs); err != nil {
		return nil, pErr("invalid AST digest output: %s", err.Error())
	}
	if funcs == nil {
		funcs = map[string]string{}
	}
	return funcs, nil
}

func pythonLogicalLinesStrict(src string) ([]logicalLine, error) {
	phys := splitLines(src)
	var out []logicalLine
	var buf strings.Builder
	baseIndent := 0
	started := false
	depth := 0
	var quote byte
	triple := false
	escaped := false
	for lineNo, line := range phys {
		if !started && quote == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		if !started {
			baseIndent = countIndent(line)
			started = true
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
		var underflow bool
		depth, quote, triple, escaped, underflow = scanPyLineStateStrict(line, depth, quote, triple, escaped)
		if underflow {
			return nil, pErr("unmatched closing bracket at line %d", lineNo+1)
		}
		if quote != 0 && !triple && !strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) {
			return nil, pErr("unterminated string literal at line %d", lineNo+1)
		}
		cont := depth > 0 || quote != 0 || strings.HasSuffix(strings.TrimRight(line, " \t"), `\`)
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
	if depth != 0 {
		return nil, pErr("unclosed bracket/parenthesis at EOF")
	}
	if buf.Len() > 0 {
		out = append(out, logicalLine{indent: baseIndent, text: strings.TrimRight(buf.String(), "\n")})
	}
	return out, nil
}

func scanPyLineStateStrict(line string, depth int, quote byte, triple, escaped bool) (int, byte, bool, bool, bool) {
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
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return 0, quote, triple, escaped, true
			}
			depth--
		}
		i++
	}
	return depth, quote, triple, escaped, false
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

func collapseCode(text string) string {
	var b strings.Builder
	n := len(text)
	for i := 0; i < n; {
		ch := text[i]
		if ch == '#' {
			for i < n && text[i] != '\n' {
				i++
			}
			if i < n {
				b.WriteByte(' ')
				i++
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			// Check for string prefix (r, b, u, f, etc.) immediately before quote.
			rawOrFmt := false
			if i > 0 {
				prev := text[i-1]
				if prev == 'r' || prev == 'R' || prev == 'f' || prev == 'F' || prev == 'b' || prev == 'B' {
					rawOrFmt = true
				}
			}
			triple := i+2 < n && text[i+1] == ch && text[i+2] == ch
			start := i
			if triple {
				i += 3
				for i < n {
					if text[i] == '\\' && i+1 < n {
						i += 2
						continue
					}
					if i+2 < n && text[i] == ch && text[i+1] == ch && text[i+2] == ch {
						i += 3
						break
					}
					i++
				}
				b.WriteString(text[start:i])
			} else {
				i++
				for i < n {
					if text[i] == '\\' && i+1 < n {
						i += 2
						continue
					}
					if text[i] == ch {
						i++
						break
					}
					i++
				}
				seg := text[start:i]
				if !rawOrFmt {
					seg = normalizePyQuotedString(seg)
				}
				b.WriteString(seg)
			}
			continue
		}
		if ch == '\n' || ch == '\r' || ch == '\t' {
			b.WriteByte(' ')
			i++
			continue
		}
		b.WriteByte(ch)
		i++
	}
	collapsed := strings.Join(strings.Fields(b.String()), " ")
	// Normalize spaces adjacent to delimiters and strip trailing commas before ), ], }.
	var out strings.Builder
	inQuote := byte(0)
	esc := false
	for i := 0; i < len(collapsed); i++ {
		ch := collapsed[i]
		if esc {
			out.WriteByte(ch)
			esc = false
			continue
		}
		if inQuote != 0 {
			out.WriteByte(ch)
			switch ch {
			case '\\':
				esc = true
			case inQuote:
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			out.WriteByte(ch)
			continue
		}
		if ch == ',' {
			// Skip trailing comma before closing delimiter.
			j := i + 1
			for j < len(collapsed) && collapsed[j] == ' ' {
				j++
			}
			if j < len(collapsed) && (collapsed[j] == ')' || collapsed[j] == ']' || collapsed[j] == '}') {
				i = j - 1
				continue
			}
		}
		if ch == ' ' {
			prev := byte(0)
			if out.Len() > 0 {
				s := out.String()
				prev = s[len(s)-1]
			}
			next := byte(0)
			if i+1 < len(collapsed) {
				next = collapsed[i+1]
			}
			if prev == '(' || prev == '[' || prev == '{' || next == ')' || next == ']' || next == '}' || next == ',' || next == ':' {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

var (
	pyDefLikeRe   = regexp.MustCompile(`^(?:async\s+)?def\b`)
	pyClassLikeRe = regexp.MustCompile(`^class\b`)
)

func extractPythonFuncBodiesPureGo(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	lines, err := pythonLogicalLinesStrict(source)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		head := firstContent(line)
		if pyDefLikeRe.MatchString(head) {
			if _, _, ok := pyHeader(line); !ok {
				return nil, pErr("invalid def header: %q", head)
			}
			norm := collapseCode(line.text)
			if !strings.Contains(norm, ":") {
				return nil, pErr("def header missing ':': %q", head)
			}
		} else if pyClassLikeRe.MatchString(head) {
			if _, _, ok := pyHeader(line); !ok {
				return nil, pErr("invalid class header: %q", head)
			}
			norm := collapseCode(line.text)
			if !strings.Contains(norm, ":") {
				return nil, pErr("class header missing ':': %q", head)
			}
		}
	}
	funcs := map[string]string{}
	counts := map[string]int{}
	var walk func(start, end, minIndent int, prefix string) int
	walk = func(start, end, minIndent int, prefix string) int {
		i := start
		for i < end {
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
	return funcs, nil
}

func fingerprintPython(kind, name string, header, body []logicalLine, headerIndent int) string {
	var b strings.Builder
	b.WriteString(kind)
	b.WriteByte('\n')
	b.WriteString(name)
	b.WriteByte('\n')
	for _, h := range header {
		b.WriteString(collapseCode(h.text))
		b.WriteByte('\n')
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
		b.WriteString(collapseCode(line.text))
		b.WriteByte('\n')
	}
	sum := hashNormalized(b.String())
	return sum
}
