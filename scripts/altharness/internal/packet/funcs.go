package packet

import (
	"alt/scripts/altharness/internal/screener"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
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
		funcs[key] = hashNormalizedLang(keep[start:end], "go")
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
		funcs[key] = hashNormalizedLang(keep[declStart:end], "rust")
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
	funcs[key] = hashNormalizedLang(span, "typescript")
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

func extractPythonFuncBodies(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	return extractPythonFuncBodiesPureGo(source)
}

// --- Pure-Go Python 3.12-3.14 AST-backed function extraction & fingerprinting ---

type pyBlockStmt struct {
	kind       string // "FunctionDef", "AsyncFunctionDef", "class", "compound", "simple"
	name       string
	decorators [][]screener.PyToken
	header     []screener.PyToken
	body       []pyBlockStmt
}

type pyBlockParser struct {
	toks []screener.PyToken
	pos  int
}

func (p *pyBlockParser) peek() screener.PyToken {
	if p.pos >= len(p.toks) {
		return screener.PyToken{Kind: screener.PyEOF}
	}
	return p.toks[p.pos]
}

func (p *pyBlockParser) peekN(n int) screener.PyToken {
	i := p.pos + n
	if i < 0 || i >= len(p.toks) {
		return screener.PyToken{Kind: screener.PyEOF}
	}
	return p.toks[i]
}

func (p *pyBlockParser) next() screener.PyToken {
	tk := p.peek()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return tk
}

func (p *pyBlockParser) parseBlock() ([]pyBlockStmt, error) {
	var stmts []pyBlockStmt
	for {
		for p.peek().Kind == screener.PyNL {
			p.next()
		}
		tk := p.peek()
		if tk.Kind == screener.PyEOF || tk.Kind == screener.PyDedent {
			return stmts, nil
		}
		if tk.Kind == screener.PyIndent {
			return nil, pErr("unexpected indent at line %d", tk.Line)
		}
		var decorators [][]screener.PyToken
		for p.peek().Kind == screener.PyOp && p.peek().Val == "@" {
			var dec []screener.PyToken
			for p.peek().Kind != screener.PyNL && p.peek().Kind != screener.PyEOF && p.peek().Kind != screener.PyDedent {
				dec = append(dec, p.next())
			}
			decorators = append(decorators, dec)
			for p.peek().Kind == screener.PyNL {
				p.next()
			}
		}
		cur := p.peek()
		if cur.Kind == screener.PyEOF || cur.Kind == screener.PyDedent {
			return nil, pErr("dangling decorator at line %d", cur.Line)
		}
		if kind, name, ok := p.matchDefOrClass(); ok {
			header, err := p.consumeUntilTopColon()
			if err != nil {
				return nil, err
			}
			body, err := p.parseSuite()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, pyBlockStmt{
				kind:       kind,
				name:       name,
				decorators: decorators,
				header:     header,
				body:       body,
			})
			continue
		}
		if len(decorators) > 0 {
			return nil, pErr("expected def or class after decorator at line %d", cur.Line)
		}
		if p.isCompoundHeader() {
			header, err := p.consumeUntilTopColon()
			if err != nil {
				return nil, err
			}
			body, err := p.parseSuite()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, pyBlockStmt{
				kind:   "compound",
				header: header,
				body:   body,
			})
			continue
		}
		simpleLine := p.consumeLine()
		stmts = append(stmts, splitSimplePyLine(simpleLine)...)
	}
}

func (p *pyBlockParser) matchDefOrClass() (kind, name string, ok bool) {
	t0 := p.peek()
	if t0.Kind != screener.PyIdent {
		return "", "", false
	}
	switch t0.Val {
	case "def":
		t1 := p.peekN(1)
		if t1.Kind == screener.PyIdent {
			return "FunctionDef", t1.Val, true
		}
	case "async":
		t1 := p.peekN(1)
		t2 := p.peekN(2)
		if t1.Kind == screener.PyIdent && t1.Val == "def" && t2.Kind == screener.PyIdent {
			return "AsyncFunctionDef", t2.Val, true
		}
	case "class":
		t1 := p.peekN(1)
		if t1.Kind == screener.PyIdent {
			return "class", t1.Val, true
		}
	}
	return "", "", false
}

func (p *pyBlockParser) isCompoundHeader() bool {
	t0 := p.peek()
	if t0.Kind != screener.PyIdent {
		return false
	}
	switch t0.Val {
	case "if", "elif", "else", "for", "while", "with", "try", "except", "finally":
		return p.findTopColonOnLine() >= 0
	case "async":
		t1 := p.peekN(1)
		if t1.Kind == screener.PyIdent && (t1.Val == "for" || t1.Val == "with") {
			return p.findTopColonOnLine() >= 0
		}
		return false
	case "match", "case":
		colonIdx := p.findTopColonOnLine()
		if colonIdx <= p.pos+1 {
			return false
		}
		depth := 0
		for i := p.pos + 1; i < colonIdx; i++ {
			tk := p.toks[i]
			if tk.Kind == screener.PyOp {
				switch tk.Val {
				case "(", "[", "{":
					depth++
				case ")", "]", "}":
					depth--
				default:
					if depth == 0 {
						if _, isAssign := pyAssignOps[tk.Val]; isAssign || tk.Val == ":=" {
							return false
						}
					}
				}
			}
		}
		if t0.Val == "match" {
			return colonIdx+1 < len(p.toks) && p.toks[colonIdx+1].Kind == screener.PyNL
		}
		return true
	default:
		return false
	}
}

func (p *pyBlockParser) findTopColonOnLine() int {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		tk := p.toks[i]
		if tk.Kind == screener.PyNL || tk.Kind == screener.PyDedent || tk.Kind == screener.PyEOF {
			return -1
		}
		if tk.Kind == screener.PyOp {
			switch tk.Val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ":":
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

func (p *pyBlockParser) consumeUntilTopColon() ([]screener.PyToken, error) {
	colonIdx := p.findTopColonOnLine()
	if colonIdx < 0 {
		return nil, pErr("missing ':' in compound header at line %d", p.peek().Line)
	}
	header := append([]screener.PyToken(nil), p.toks[p.pos:colonIdx+1]...)
	p.pos = colonIdx + 1
	return header, nil
}

func (p *pyBlockParser) consumeLine() []screener.PyToken {
	var line []screener.PyToken
	for p.peek().Kind != screener.PyNL && p.peek().Kind != screener.PyEOF && p.peek().Kind != screener.PyDedent {
		line = append(line, p.next())
	}
	if p.peek().Kind == screener.PyNL {
		p.next()
	}
	return line
}

func (p *pyBlockParser) parseSuite() ([]pyBlockStmt, error) {
	if p.peek().Kind == screener.PyNL {
		p.next()
		if p.peek().Kind != screener.PyIndent {
			return nil, pErr("expected indented block at line %d", p.peek().Line)
		}
		p.next()
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind == screener.PyDedent {
			p.next()
		}
		if len(body) == 0 {
			return nil, pErr("empty indented block at line %d", p.peek().Line)
		}
		return body, nil
	}
	line := p.consumeLine()
	stmts := splitSimplePyLine(line)
	if len(stmts) == 0 {
		return nil, pErr("empty single-line suite at line %d", p.peek().Line)
	}
	return stmts, nil
}

func splitSimplePyLine(line []screener.PyToken) []pyBlockStmt {
	var out []pyBlockStmt
	depth := 0
	start := 0
	for i, tk := range line {
		if tk.Kind == screener.PyOp {
			switch tk.Val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ";":
				if depth == 0 {
					if part := line[start:i]; len(part) > 0 {
						out = append(out, pyBlockStmt{kind: "simple", header: part})
					}
					start = i + 1
				}
			}
		}
	}
	if start < len(line) {
		out = append(out, pyBlockStmt{kind: "simple", header: line[start:]})
	}
	return out
}

func extractPythonFuncBodiesPureGo(source string) (map[string]string, error) {
	if strings.TrimSpace(source) == "" {
		return map[string]string{}, nil
	}
	if err := screener.ValidatePythonSyntax(source); err != nil {
		return nil, pErr("%s", err.Error())
	}
	toks, err := screener.TokenizePython(source)
	if err != nil {
		return nil, pErr("%s", err.Error())
	}
	parser := &pyBlockParser{toks: toks}
	stmts, err := parser.parseBlock()
	if err != nil {
		return nil, err
	}
	funcs := map[string]string{}
	counts := map[string]int{}
	var walk func(block []pyBlockStmt, prefix string) error
	walk = func(block []pyBlockStmt, prefix string) error {
		for _, stmt := range block {
			switch stmt.kind {
			case "FunctionDef", "AsyncFunctionDef":
				base := stmt.name
				if prefix != "" {
					base = prefix + "." + stmt.name
				}
				counts[base]++
				key := base
				if counts[base] > 1 {
					key = fmt.Sprintf("%s#%d", base, counts[base])
				}
				fp, err := fingerprintPyFuncStmt(stmt)
				if err != nil {
					return err
				}
				funcs[key] = fp
				if err := walk(stmt.body, base); err != nil {
					return err
				}
			case "class":
				cls := stmt.name
				if prefix != "" {
					cls = prefix + "." + stmt.name
				}
				if err := walk(stmt.body, cls); err != nil {
					return err
				}
			default:
				if len(stmt.body) > 0 {
					if err := walk(stmt.body, prefix); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(stmts, ""); err != nil {
		return nil, err
	}
	return funcs, nil
}

// --- Canonical Python token normalization & unambiguous hashing ---

type pyNormKind int

const (
	normIdent pyNormKind = iota
	normNum
	normStr
	normBytes
	normFStr
	normTStr
	normOp
)

type pyFPart struct {
	isExpr bool
	text   string
}

type pyNormTok struct {
	kind   pyNormKind
	text   string
	fParts []pyFPart
}

var pyNonCallKeywords = map[string]struct{}{
	"return": {}, "yield": {}, "assert": {}, "raise": {},
	"if": {}, "elif": {}, "while": {}, "for": {}, "in": {},
	"not": {}, "and": {}, "or": {}, "is": {}, "del": {},
	"await": {}, "lambda": {}, "as": {}, "from": {},
	"except": {}, "with": {}, "case": {}, "match": {},
}

var pyAssignOps = map[string]struct{}{
	"=": {}, "+=": {}, "-=": {}, "*=": {}, "/=": {}, "//=": {},
	"%=": {}, "**=": {}, "&=": {}, "|=": {}, "^=": {}, ">>=": {},
	"<<=": {}, "@=": {},
}

var pyPrefixExprKeywords = map[string]struct{}{
	"return": {}, "yield": {}, "del": {}, "await": {},
}

func fingerprintPyFuncStmt(stmt pyBlockStmt) (string, error) {
	var b strings.Builder
	writeField(&b, "K", stmt.kind)
	writeField(&b, "N", stmt.name)
	for _, dec := range stmt.decorators {
		norm, err := normalizePyStmt(dec, "simple")
		if err != nil {
			return "", err
		}
		writeNormStmt(&b, "@", norm)
	}
	hdrNorm, err := normalizePyStmt(stmt.header, stmt.kind)
	if err != nil {
		return "", err
	}
	writeNormStmt(&b, "H", hdrNorm)
	if err := writePyBodyFingerprint(&b, stmt.body); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func writePyBodyFingerprint(b *strings.Builder, body []pyBlockStmt) error {
	b.WriteString("I\x00")
	for _, stmt := range body {
		switch stmt.kind {
		case "FunctionDef", "AsyncFunctionDef":
			// Nested functions are fingerprinted independently; do not double-count outer.
			continue
		case "class":
			for _, dec := range stmt.decorators {
				norm, err := normalizePyStmt(dec, "simple")
				if err != nil {
					return err
				}
				writeNormStmt(b, "@", norm)
			}
			hdrNorm, err := normalizePyStmt(stmt.header, "class")
			if err != nil {
				return err
			}
			writeNormStmt(b, "C", hdrNorm)
			if err := writePyBodyFingerprint(b, stmt.body); err != nil {
				return err
			}
		case "compound":
			hdrNorm, err := normalizePyStmt(stmt.header, "compound")
			if err != nil {
				return err
			}
			writeNormStmt(b, "S", hdrNorm)
			if err := writePyBodyFingerprint(b, stmt.body); err != nil {
				return err
			}
		default:
			norm, err := normalizePyStmt(stmt.header, "simple")
			if err != nil {
				return err
			}
			if len(norm) > 0 {
				writeNormStmt(b, "S", norm)
			}
		}
	}
	b.WriteString("D\x00")
	return nil
}

func writeField(b *strings.Builder, tag, val string) {
	b.WriteString(tag)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(len(val)))
	b.WriteByte(':')
	b.WriteString(val)
	b.WriteByte(0)
}

func writeNormStmt(b *strings.Builder, tag string, toks []pyNormTok) {
	b.WriteString(tag)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(len(toks)))
	b.WriteByte(0)
	for _, tk := range toks {
		b.WriteString(strconv.Itoa(int(tk.kind)))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(len(tk.text)))
		b.WriteByte(':')
		b.WriteString(tk.text)
		b.WriteByte(0)
	}
	b.WriteByte(0x1e)
}

func serializeNormTokens(toks []pyNormTok) string {
	var b strings.Builder
	writeNormStmt(&b, "E", toks)
	return b.String()
}

func normalizePyStmt(raw []screener.PyToken, stmtKind string) ([]pyNormTok, error) {
	toks, err := convertPyTokens(raw)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, nil
	}
	toks = mergeAdjacentPyNormStrings(toks)
	toks = stripTrailingPyCommasNorm(toks)
	toks = mergeAdjacentPyNormStrings(toks)
	return normalizePyStmtParens(toks, stmtKind), nil
}

func normalizePyExprSlice(raw []screener.PyToken) ([]pyNormTok, error) {
	toks, err := convertPyTokens(raw)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, nil
	}
	toks = mergeAdjacentPyNormStrings(toks)
	toks = stripTrailingPyCommasNorm(toks)
	toks = mergeAdjacentPyNormStrings(toks)
	return stripOuterGroupingParensNorm(toks), nil
}

func convertPyTokens(raw []screener.PyToken) ([]pyNormTok, error) {
	out := make([]pyNormTok, 0, len(raw))
	for _, tk := range raw {
		switch tk.Kind {
		case screener.PyIdent:
			out = append(out, pyNormTok{kind: normIdent, text: tk.Val})
		case screener.PyNum:
			out = append(out, pyNormTok{kind: normNum, text: normalizePyNumber(tk.Val)})
		case screener.PyOp:
			out = append(out, pyNormTok{kind: normOp, text: tk.Val})
		case screener.PyStr:
			nt, err := normalizePyStrLit(tk.Val)
			if err != nil {
				return nil, err
			}
			out = append(out, nt)
		}
	}
	return out, nil
}

func normalizePyNumber(raw string) string {
	cleaned := strings.ReplaceAll(raw, "_", "")
	return strings.ToLower(cleaned)
}

func splitPyStrPrefixAndBody(raw string) (prefix, body string, quote byte, triple bool) {
	i := 0
	for i < len(raw) && raw[i] != '"' && raw[i] != '\'' {
		i++
	}
	prefix = strings.ToLower(raw[:i])
	rest := raw[i:]
	if len(rest) < 2 {
		return prefix, rest, 0, false
	}
	quote = rest[0]
	if len(rest) >= 6 && rest[1] == quote && rest[2] == quote &&
		rest[len(rest)-1] == quote && rest[len(rest)-2] == quote && rest[len(rest)-3] == quote {
		return prefix, rest[3 : len(rest)-3], quote, true
	}
	return prefix, rest[1 : len(rest)-1], quote, false
}

func normalizePyStrLit(raw string) (pyNormTok, error) {
	prefix, body, _, _ := splitPyStrPrefixAndBody(raw)
	isRaw := strings.ContainsRune(prefix, 'r')
	isBytes := strings.ContainsRune(prefix, 'b')
	isF := strings.ContainsRune(prefix, 'f')
	isT := strings.ContainsRune(prefix, 't')

	if isBytes {
		decoded := body
		if !isRaw {
			decoded = decodePyEscapes(body)
		}
		return pyNormTok{kind: normBytes, text: decoded}, nil
	}
	if isF || isT {
		parts, err := parsePyFStringParts(body, isRaw)
		if err != nil {
			return pyNormTok{}, err
		}
		kind := normFStr
		if isT {
			kind = normTStr
		}
		return pyNormTok{kind: kind, text: serializeFParts(parts), fParts: parts}, nil
	}
	decoded := body
	if !isRaw {
		decoded = decodePyEscapes(body)
	}
	parts := []pyFPart{{isExpr: false, text: decoded}}
	return pyNormTok{kind: normStr, text: decoded, fParts: parts}, nil
}

func decodePyEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	n := len(s)
	for i := 0; i < n; {
		if s[i] != '\\' || i+1 >= n {
			b.WriteByte(s[i])
			i++
			continue
		}
		nxt := s[i+1]
		switch nxt {
		case '\n':
			i += 2
		case '\\':
			b.WriteByte('\\')
			i += 2
		case '\'':
			b.WriteByte('\'')
			i += 2
		case '"':
			b.WriteByte('"')
			i += 2
		case 'a':
			b.WriteByte('\a')
			i += 2
		case 'b':
			b.WriteByte('\b')
			i += 2
		case 'f':
			b.WriteByte('\f')
			i += 2
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'v':
			b.WriteByte('\v')
			i += 2
		case 'x':
			if i+4 <= n {
				if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteRune(rune(v))
					i += 4
					continue
				}
			}
			b.WriteString(s[i : i+2])
			i += 2
		case 'u':
			if i+6 <= n {
				if v, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
					b.WriteRune(rune(v))
					i += 6
					continue
				}
			}
			b.WriteString(s[i : i+2])
			i += 2
		case 'U':
			if i+10 <= n {
				if v, err := strconv.ParseUint(s[i+2:i+10], 16, 32); err == nil && utf8.ValidRune(rune(v)) {
					b.WriteRune(rune(v))
					i += 10
					continue
				}
			}
			b.WriteString(s[i : i+2])
			i += 2
		case '0', '1', '2', '3', '4', '5', '6', '7':
			end := i + 2
			for end < n && end < i+4 && s[end] >= '0' && s[end] <= '7' {
				end++
			}
			if v, err := strconv.ParseUint(s[i+1:end], 8, 16); err == nil {
				b.WriteRune(rune(v))
				i = end
				continue
			}
			b.WriteString(s[i : i+2])
			i += 2
		default:
			b.WriteByte('\\')
			b.WriteByte(nxt)
			i += 2
		}
	}
	return b.String()
}

func parsePyFStringParts(body string, isRaw bool) ([]pyFPart, error) {
	var parts []pyFPart
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() == 0 {
			return
		}
		text := lit.String()
		if !isRaw {
			text = decodePyEscapes(text)
		}
		if text != "" {
			if len(parts) > 0 && !parts[len(parts)-1].isExpr {
				parts[len(parts)-1].text += text
			} else {
				parts = append(parts, pyFPart{isExpr: false, text: text})
			}
		}
		lit.Reset()
	}

	n := len(body)
	for i := 0; i < n; {
		ch := body[i]
		if ch == '\\' && i+1 < n {
			if !isRaw && body[i+1] == 'N' && i+2 < n && body[i+2] == '{' {
				j := i + 3
				for j < n && body[j] != '}' {
					j++
				}
				if j < n && body[j] == '}' {
					lit.WriteString(body[i : j+1])
					i = j + 1
					continue
				}
			}
			if body[i+1] == '\\' || body[i+1] == '\'' || body[i+1] == '"' || body[i+1] == '\n' {
				lit.WriteString(body[i : i+2])
				i += 2
				continue
			}
			lit.WriteByte(ch)
			i++
			continue
		}
		if ch == '{' {
			if i+1 < n && body[i+1] == '{' {
				flushLit()
				if len(parts) > 0 && !parts[len(parts)-1].isExpr {
					parts[len(parts)-1].text += "{"
				} else {
					parts = append(parts, pyFPart{isExpr: false, text: "{"})
				}
				i += 2
				continue
			}
			flushLit()
			exprStart := i + 1
			exprEnd, err := scanFExprEnd(body, exprStart)
			if err != nil {
				return nil, err
			}
			exprRaw := body[exprStart:exprEnd]
			i = exprEnd

			conv := ""
			if i < n && body[i] == '!' {
				if i+1 < n {
					conv = body[i : i+2]
					i += 2
				} else {
					return nil, pErr("unterminated f-string conversion")
				}
			}
			specCanon := ""
			if i < n && body[i] == ':' {
				specStart := i + 1
				specEnd, err := scanFSpecEnd(body, specStart)
				if err != nil {
					return nil, err
				}
				specParts, err := parsePyFStringParts(body[specStart:specEnd], isRaw)
				if err != nil {
					return nil, err
				}
				specCanon = serializeFParts(specParts)
				i = specEnd
			}
			if i >= n || body[i] != '}' {
				return nil, pErr("unterminated f-string expression")
			}
			i++

			// Handle PEP 3.8 debug f-strings like f"{x=}" or f"{x = }".
			evalSrc := exprRaw
			if eqIdx, isDebug := findFStringDebugEq(exprRaw); isDebug {
				debugLit := exprRaw[:eqIdx+1]
				if len(parts) > 0 && !parts[len(parts)-1].isExpr {
					parts[len(parts)-1].text += debugLit
				} else {
					parts = append(parts, pyFPart{isExpr: false, text: debugLit})
				}
				evalSrc = exprRaw[:eqIdx]
				if conv == "" && specCanon == "" {
					conv = "!r"
				}
			}

			exprToks, err := screener.TokenizePythonExpr(evalSrc)
			if err != nil {
				return nil, pErr("invalid f-string expression: %s", err.Error())
			}
			normExpr, err := normalizePyExprSlice(exprToks)
			if err != nil {
				return nil, err
			}
			var eb strings.Builder
			writeField(&eb, "X", serializeNormTokens(normExpr))
			writeField(&eb, "C", conv)
			writeField(&eb, "F", specCanon)
			parts = append(parts, pyFPart{isExpr: true, text: eb.String()})
			continue
		}
		if ch == '}' && i+1 < n && body[i+1] == '}' {
			flushLit()
			if len(parts) > 0 && !parts[len(parts)-1].isExpr {
				parts[len(parts)-1].text += "}"
			} else {
				parts = append(parts, pyFPart{isExpr: false, text: "}"})
			}
			i += 2
			continue
		}
		lit.WriteByte(ch)
		i++
	}
	flushLit()
	return parts, nil
}

func findFStringDebugEq(exprRaw string) (int, bool) {
	trimmed := strings.TrimRight(exprRaw, " \t\r\n")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '=' {
		return -1, false
	}
	if len(trimmed) >= 2 {
		prev := trimmed[len(trimmed)-2]
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' || prev == ':' {
			return -1, false
		}
	}
	return len(trimmed) - 1, true
}

func scanFExprEnd(src string, i int) (int, error) {
	var stack []byte
	for i < len(src) {
		if end, ok, err := skipPyNestedString(src, i); ok || err != nil {
			if err != nil {
				return 0, err
			}
			i = end
			continue
		}
		c := src[i]
		switch c {
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case '(', '[', '{':
			stack = append(stack, c)
			i++
		case ')', ']', '}':
			if c == '}' && len(stack) == 0 {
				return i, nil
			}
			if len(stack) == 0 {
				return 0, pErr("unmatched closing bracket %q in f-string", string(c))
			}
			stack = stack[:len(stack)-1]
			i++
		case '!':
			if i+1 < len(src) && src[i+1] == '=' {
				i += 2
				continue
			}
			if len(stack) == 0 {
				return i, nil
			}
			i++
		case ':':
			if len(stack) == 0 {
				return i, nil
			}
			i++
		case '\\':
			i += 2
		default:
			_, sz := utf8.DecodeRuneInString(src[i:])
			if sz < 1 {
				sz = 1
			}
			i += sz
		}
	}
	return 0, pErr("unterminated f-string expression")
}

func scanFSpecEnd(src string, i int) (int, error) {
	for i < len(src) {
		c := src[i]
		if c == '\\' && i+1 < len(src) {
			i += 2
			continue
		}
		if c == '{' {
			endExpr, err := scanFExprEnd(src, i+1)
			if err != nil {
				return 0, err
			}
			i = endExpr
			if i < len(src) && src[i] == '!' && i+1 < len(src) {
				i += 2
			}
			if i < len(src) && src[i] == ':' {
				endSub, err := scanFSpecEnd(src, i+1)
				if err != nil {
					return 0, err
				}
				i = endSub
			}
			if i >= len(src) || src[i] != '}' {
				return 0, pErr("unterminated nested f-string format spec")
			}
			i++
			continue
		}
		if c == '}' {
			return i, nil
		}
		i++
	}
	return 0, pErr("unterminated f-string format spec")
}

func skipPyNestedString(src string, i int) (int, bool, error) {
	if i >= len(src) {
		return 0, false, nil
	}
	startQuote := -1
	prefix := ""
	if src[i] == '"' || src[i] == '\'' {
		startQuote = i
	} else {
		if i > 0 {
			prev, _ := utf8.DecodeLastRuneInString(src[:i])
			if prev == '_' || isUnicodeAlnum(prev) {
				return 0, false, nil
			}
		}
		for k := 2; k >= 1; k-- {
			if i+k < len(src) && (src[i+k] == '"' || src[i+k] == '\'') && isValidPyStrPrefix(src[i:i+k]) {
				prefix = strings.ToLower(src[i : i+k])
				startQuote = i + k
				break
			}
		}
	}
	if startQuote < 0 {
		return 0, false, nil
	}
	q := src[startQuote]
	triple := startQuote+2 < len(src) && src[startQuote+1] == q && src[startQuote+2] == q
	endMark := string(q)
	j := startQuote + 1
	if triple {
		endMark = strings.Repeat(endMark, 3)
		j = startQuote + 3
	}
	isF := strings.ContainsAny(prefix, "ft")
	for j < len(src) {
		if strings.HasPrefix(src[j:], endMark) {
			return j + len(endMark), true, nil
		}
		if src[j] == '\\' && j+1 < len(src) {
			j += 2
			continue
		}
		if isF && src[j] == '{' {
			if j+1 < len(src) && src[j+1] == '{' {
				j += 2
				continue
			}
			endExpr, err := scanFExprEnd(src, j+1)
			if err != nil {
				return 0, false, err
			}
			j = endExpr
			if j < len(src) && src[j] == '!' && j+1 < len(src) {
				j += 2
			}
			if j < len(src) && src[j] == ':' {
				endSpec, err := scanFSpecEnd(src, j+1)
				if err != nil {
					return 0, false, err
				}
				j = endSpec
			}
			if j >= len(src) || src[j] != '}' {
				return 0, false, pErr("unterminated nested f-string expression")
			}
			j++
			continue
		}
		j++
	}
	return 0, false, pErr("unterminated string inside f-string")
}

func isValidPyStrPrefix(s string) bool {
	low := strings.ToLower(s)
	if len(low) == 0 || len(low) > 2 {
		return false
	}
	seen := map[byte]bool{}
	for i := 0; i < len(low); i++ {
		c := low[i]
		if c != 'r' && c != 'f' && c != 'b' && c != 'u' && c != 't' {
			return false
		}
		if seen[c] {
			return false
		}
		seen[c] = true
	}
	if seen['u'] && len(low) > 1 {
		return false
	}
	if seen['b'] && (seen['f'] || seen['t']) {
		return false
	}
	if seen['f'] && seen['t'] {
		return false
	}
	return true
}

func serializeFParts(parts []pyFPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.isExpr {
			writeField(&b, "E", p.text)
		} else if p.text != "" {
			writeField(&b, "L", p.text)
		}
	}
	return b.String()
}

func mergeFParts(a, b []pyFPart) []pyFPart {
	out := make([]pyFPart, 0, len(a)+len(b))
	appendPart := func(p pyFPart) {
		if !p.isExpr && p.text == "" {
			return
		}
		if !p.isExpr && len(out) > 0 && !out[len(out)-1].isExpr {
			out[len(out)-1].text += p.text
			return
		}
		out = append(out, p)
	}
	for _, p := range a {
		appendPart(p)
	}
	for _, p := range b {
		appendPart(p)
	}
	return out
}

func mergeAdjacentPyNormStrings(toks []pyNormTok) []pyNormTok {
	if len(toks) < 2 {
		return toks
	}
	out := make([]pyNormTok, 0, len(toks))
	for _, tok := range toks {
		if len(out) > 0 {
			prev := &out[len(out)-1]
			if prev.kind == normBytes && tok.kind == normBytes {
				prev.text += tok.text
				continue
			}
			prevStrFam := prev.kind == normStr || prev.kind == normFStr
			curStrFam := tok.kind == normStr || tok.kind == normFStr
			if prevStrFam && curStrFam {
				combined := mergeFParts(prev.fParts, tok.fParts)
				if prev.kind == normFStr || tok.kind == normFStr {
					prev.kind = normFStr
					prev.fParts = combined
					prev.text = serializeFParts(combined)
				} else {
					text := ""
					if len(combined) == 1 && !combined[0].isExpr {
						text = combined[0].text
					}
					prev.kind = normStr
					prev.text = text
					prev.fParts = []pyFPart{{isExpr: false, text: text}}
				}
				continue
			}
		}
		out = append(out, tok)
	}
	return out
}

func matchingNormClose(toks []pyNormTok, openIdx int) int {
	if openIdx < 0 || openIdx >= len(toks) || toks[openIdx].kind != normOp {
		return -1
	}
	open := toks[openIdx].text
	var closeTok string
	switch open {
	case "[":
		closeTok = "]"
	case "{":
		closeTok = "}"
	case "(":
		closeTok = ")"
	default:
		return -1
	}
	depth := 0
	for i := openIdx; i < len(toks); i++ {
		if toks[i].kind != normOp {
			continue
		}
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

func stripTrailingPyCommasNorm(toks []pyNormTok) []pyNormTok {
	type frame struct {
		open       string
		isCallLike bool
		commaCount int
	}
	var stack []frame
	drop := make([]bool, len(toks))
	for i, tok := range toks {
		if tok.kind != normOp {
			continue
		}
		switch tok.text {
		case "(":
			callLike := false
			if i > 0 {
				prev := toks[i-1]
				if prev.kind == normIdent {
					if i >= 2 && toks[i-2].kind == normOp && toks[i-2].text == "." {
						// Attribute call like re.match(pat,) or obj.case(x,).
						callLike = true
					} else if prev.text == "import" {
						// `from m import (A,)` is an import list, never a 1-tuple.
						callLike = true
					} else if prev.text == "with" {
						// `with (open(a) as fa,):` is a parenthesized with-item list.
						if closeIdx := matchingNormClose(toks, i); closeIdx > i && closeIdx+1 < len(toks) && toks[closeIdx+1].kind == normOp && toks[closeIdx+1].text == ":" {
							callLike = true
						}
					} else if prev.text == "match" || prev.text == "case" {
						// Soft keywords: only non-call when opening a `match (...):` / `case (...):` statement.
						isStmtHead := i == 1 && len(toks) > 0 && toks[len(toks)-1].kind == normOp && toks[len(toks)-1].text == ":"
						callLike = !isStmtHead
					} else if _, kw := pyNonCallKeywords[prev.text]; !kw {
						callLike = true
					}
				} else if prev.kind == normOp && (prev.text == ")" || prev.text == "]" || prev.text == "}") {
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
				if i > 0 && toks[i-1].kind == normOp && toks[i-1].text == "," {
					if tok.text != ")" || top.isCallLike || top.commaCount > 1 {
						drop[i-1] = true
					}
				}
			}
		}
	}
	out := make([]pyNormTok, 0, len(toks))
	for i, tok := range toks {
		if !drop[i] {
			out = append(out, tok)
		}
	}
	return out
}

func stripOuterGroupingParensNorm(toks []pyNormTok) []pyNormTok {
	for len(toks) >= 2 && toks[0].kind == normOp && toks[0].text == "(" && matchingNormClose(toks, 0) == len(toks)-1 {
		inner := toks[1 : len(toks)-1]
		if len(inner) == 0 {
			break
		}
		depth := 0
		keep := false
		for _, tok := range inner {
			if tok.kind == normOp {
				switch tok.text {
				case "(", "[", "{":
					depth++
				case ")", "]", "}":
					depth--
				default:
					if depth == 0 && (tok.text == "," || tok.text == ":=") {
						keep = true
					}
				}
			} else if depth == 0 && tok.kind == normIdent && (tok.text == "for" || tok.text == "async" || tok.text == "yield") {
				keep = true
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

func findTopLevelOp(toks []pyNormTok, op string) int {
	depth := 0
	for i, tk := range toks {
		if tk.kind == normOp {
			switch tk.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			default:
				if depth == 0 && tk.text == op {
					return i
				}
			}
		}
	}
	return -1
}

func findTopLevelIdent(toks []pyNormTok, ident string) int {
	depth := 0
	for i, tk := range toks {
		if tk.kind == normOp {
			switch tk.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			}
		} else if depth == 0 && tk.kind == normIdent && tk.text == ident {
			return i
		}
	}
	return -1
}

func normalizePyStmtParens(toks []pyNormTok, stmtKind string) []pyNormTok {
	if len(toks) == 0 {
		return toks
	}

	// def / async def header: strip grouping parens on return annotation `-> (T):`
	// and on parameter annotations / defaults inside `(...)`.
	if stmtKind == "FunctionDef" || stmtKind == "AsyncFunctionDef" {
		toks = normalizePyDefHeader(toks)
		return toks
	}

	// Compound headers ending with ':'.
	last := toks[len(toks)-1]
	if last.kind == normOp && last.text == ":" && toks[0].kind == normIdent {
		kwIdx := 0
		if toks[0].text == "async" && len(toks) > 2 && toks[1].kind == normIdent {
			kwIdx = 1
		}
		kw := toks[kwIdx].text
		switch kw {
		case "if", "elif", "while", "match":
			cond := stripOuterGroupingParensNorm(toks[kwIdx+1 : len(toks)-1])
			return joinToks(toks[:kwIdx+1], cond, toks[len(toks)-1:])
		case "with":
			rest := toks[kwIdx+1 : len(toks)-1]
			if len(rest) >= 2 && rest[0].kind == normOp && rest[0].text == "(" && matchingNormClose(rest, 0) == len(rest)-1 {
				rest = rest[1 : len(rest)-1]
			}
			return joinToks(toks[:kwIdx+1], rest, toks[len(toks)-1:])
		case "for":
			inOff := findTopLevelIdent(toks[kwIdx+1:len(toks)-1], "in")
			if inOff >= 0 {
				inIdx := kwIdx + 1 + inOff
				iter := stripOuterGroupingParensNorm(toks[inIdx+1 : len(toks)-1])
				return joinToks(toks[:inIdx+1], iter, toks[len(toks)-1:])
			}
		case "case":
			body := toks[kwIdx+1 : len(toks)-1]
			if ifOff := findTopLevelIdent(body, "if"); ifOff >= 0 {
				pat := stripOuterGroupingParensNorm(body[:ifOff])
				guard := stripOuterGroupingParensNorm(body[ifOff+1:])
				return joinToks(toks[:kwIdx+1], pat, []pyNormTok{body[ifOff]}, guard, toks[len(toks)-1:])
			}
			pat := stripOuterGroupingParensNorm(body)
			return joinToks(toks[:kwIdx+1], pat, toks[len(toks)-1:])
		case "except":
			start := kwIdx + 1
			if start < len(toks)-1 && toks[start].kind == normOp && toks[start].text == "*" {
				start++
			}
			body := toks[start : len(toks)-1]
			if asOff := findTopLevelIdent(body, "as"); asOff >= 0 {
				exc := stripOuterGroupingParensNorm(body[:asOff])
				return joinToks(toks[:start], exc, body[asOff:], toks[len(toks)-1:])
			}
			exc := stripOuterGroupingParensNorm(body)
			return joinToks(toks[:start], exc, toks[len(toks)-1:])
		}
	}

	// `from m import (A, B)` -> `from m import A, B`.
	if toks[0].kind == normIdent && toks[0].text == "from" {
		if impIdx := findTopLevelIdent(toks[1:], "import"); impIdx >= 0 {
			impPos := 1 + impIdx
			after := toks[impPos+1:]
			if len(after) >= 2 && after[0].kind == normOp && after[0].text == "(" && matchingNormClose(after, 0) == len(after)-1 {
				return joinToks(toks[:impPos+1], after[1:len(after)-1])
			}
		}
	}

	// Prefix keyword statements: return, yield [from], raise [from], assert, del, await.
	if toks[0].kind == normIdent {
		switch toks[0].text {
		case "yield":
			if len(toks) > 2 && toks[1].kind == normIdent && toks[1].text == "from" {
				rhs := stripOuterGroupingParensNorm(toks[2:])
				return joinToks(toks[:2], rhs)
			}
			if len(toks) > 1 {
				rhs := stripOuterGroupingParensNorm(toks[1:])
				return joinToks(toks[:1], rhs)
			}
		case "raise":
			if len(toks) > 1 {
				rest := toks[1:]
				if fromOff := findTopLevelIdent(rest, "from"); fromOff >= 0 {
					lhs := stripOuterGroupingParensNorm(rest[:fromOff])
					rhs := stripOuterGroupingParensNorm(rest[fromOff+1:])
					return joinToks(toks[:1], lhs, []pyNormTok{rest[fromOff]}, rhs)
				}
				return joinToks(toks[:1], stripOuterGroupingParensNorm(rest))
			}
		case "assert":
			if len(toks) > 1 {
				rest := toks[1:]
				if commaOff := findTopLevelOp(rest, ","); commaOff >= 0 {
					cond := stripOuterGroupingParensNorm(rest[:commaOff])
					msg := stripOuterGroupingParensNorm(rest[commaOff+1:])
					return joinToks(toks[:1], cond, []pyNormTok{rest[commaOff]}, msg)
				}
				return joinToks(toks[:1], stripOuterGroupingParensNorm(rest))
			}
		default:
			if _, ok := pyPrefixExprKeywords[toks[0].text]; ok && len(toks) > 1 {
				return joinToks(toks[:1], stripOuterGroupingParensNorm(toks[1:]))
			}
		}
	}

	// Top-level assignment / annotated assignment: `lhs = (expr)`, `lhs: (T) = (expr)`, `lhs += (expr)`.
	depth := 0
	colonIdx := -1
	for i, tok := range toks {
		if tok.kind == normOp {
			switch tok.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			default:
				if depth == 0 {
					if tok.text == ":" && colonIdx < 0 {
						colonIdx = i
					} else if _, isAssign := pyAssignOps[tok.text]; isAssign && i+1 < len(toks) {
						rhs := stripOuterGroupingParensNorm(toks[i+1:])
						if colonIdx >= 0 && colonIdx < i {
							ann := stripOuterGroupingParensNorm(toks[colonIdx+1 : i])
							return joinToks(toks[:colonIdx+1], ann, toks[i:i+1], rhs)
						}
						return joinToks(toks[:i+1], rhs)
					}
				}
			}
		}
	}
	if colonIdx >= 0 && colonIdx+1 < len(toks) {
		ann := stripOuterGroupingParensNorm(toks[colonIdx+1:])
		return joinToks(toks[:colonIdx+1], ann)
	}
	return stripOuterGroupingParensNorm(toks)
}

func normalizePyDefHeader(toks []pyNormTok) []pyNormTok {
	// Normalize parameter annotations & defaults inside the main `(...)` parameter list.
	paramOpen := -1
	for i := 0; i < len(toks); i++ {
		if toks[i].kind == normOp && toks[i].text == "[" {
			if closeIdx := matchingNormClose(toks, i); closeIdx > i {
				i = closeIdx
			}
			continue
		}
		if toks[i].kind == normOp && toks[i].text == "(" {
			paramOpen = i
			break
		}
	}
	if paramOpen >= 0 {
		if paramClose := matchingNormClose(toks, paramOpen); paramClose > paramOpen {
			normParams := normalizePyParamList(toks[paramOpen+1 : paramClose])
			toks = joinToks(toks[:paramOpen+1], normParams, toks[paramClose:])
		}
	}
	// Strip outer grouping parens on return type annotation `-> (RetType):`.
	arrowIdx := findTopLevelOp(toks, "->")
	if arrowIdx >= 0 && toks[len(toks)-1].kind == normOp && toks[len(toks)-1].text == ":" {
		retAnn := stripOuterGroupingParensNorm(toks[arrowIdx+1 : len(toks)-1])
		toks = joinToks(toks[:arrowIdx+1], retAnn, toks[len(toks)-1:])
	}
	return toks
}

func normalizePyParamList(params []pyNormTok) []pyNormTok {
	if len(params) == 0 {
		return params
	}
	var out []pyNormTok
	depth := 0
	start := 0
	flush := func(end int, trailingComma *pyNormTok) {
		seg := params[start:end]
		if len(seg) > 0 {
			out = append(out, normalizePySingleParam(seg)...)
		}
		if trailingComma != nil {
			out = append(out, *trailingComma)
		}
	}
	for i, tk := range params {
		if tk.kind == normOp {
			switch tk.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			case ",":
				if depth == 0 {
					c := tk
					flush(i, &c)
					start = i + 1
				}
			}
		}
	}
	if start < len(params) {
		flush(len(params), nil)
	}
	return out
}

func normalizePySingleParam(seg []pyNormTok) []pyNormTok {
	colonIdx := findTopLevelOp(seg, ":")
	eqIdx := findTopLevelOp(seg, "=")
	if colonIdx >= 0 && eqIdx > colonIdx {
		ann := stripOuterGroupingParensNorm(seg[colonIdx+1 : eqIdx])
		defVal := stripOuterGroupingParensNorm(seg[eqIdx+1:])
		return joinToks(seg[:colonIdx+1], ann, seg[eqIdx:eqIdx+1], defVal)
	}
	if colonIdx >= 0 {
		ann := stripOuterGroupingParensNorm(seg[colonIdx+1:])
		return joinToks(seg[:colonIdx+1], ann)
	}
	if eqIdx >= 0 {
		defVal := stripOuterGroupingParensNorm(seg[eqIdx+1:])
		return joinToks(seg[:eqIdx+1], defVal)
	}
	return seg
}

func joinToks(slices ...[]pyNormTok) []pyNormTok {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	out := make([]pyNormTok, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}
