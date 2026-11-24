package screener

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	skOther = iota
	skFunc
	skClass
	skIf
	skFor
	skWhile
	skTry
	skWith
	skMatch
	skReturn
	skRaise
	skAssign
	skAnn
	skAug
	skExpr
	skAssert
)

type px struct {
	op   string
	lit  string
	kids []*px
}

type pyHandler struct {
	names []string
	body  []pyStmt
}

type pyCase struct {
	guard *px
	body  []pyStmt
}

type pyStmt struct {
	kind      int
	name      string
	async     bool
	test      *px
	value     *px
	ann       *px
	body      []pyStmt
	orelse    []pyStmt
	final     []pyStmt
	handlers  []pyHandler
	withExprs []*px
	cases     []pyCase
}

type pyParser struct {
	toks []pyTok
	pos  int
}

func parsePython(src string) ([]pyStmt, error) {
	toks, err := tokenizePy(src)
	if err != nil {
		return nil, err
	}
	p := &pyParser{toks: toks}
	var stmts []pyStmt
	for p.peek().kind != pyEOF {
		switch p.peek().kind {
		case pyNL:
			p.next()
			continue
		case pyIndent, pyDedent:
			return nil, p.errorf("unexpected indent")
		}
		part, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, part...)
	}
	return stmts, nil
}

func (p *pyParser) peek() pyTok {
	if p.pos >= len(p.toks) {
		return pyTok{kind: pyEOF}
	}
	return p.toks[p.pos]
}

func (p *pyParser) peekN(n int) pyTok {
	i := p.pos + n
	if i < 0 || i >= len(p.toks) {
		return pyTok{kind: pyEOF}
	}
	return p.toks[i]
}

func (p *pyParser) next() pyTok {
	tk := p.peek()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return tk
}

func (p *pyParser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.peek().line, fmt.Sprintf(format, args...))
}

func (p *pyParser) peekIdent(name string) bool {
	tk := p.peek()
	return tk.kind == pyIdent && tk.val == name
}

func (p *pyParser) peekOp(val string) bool {
	tk := p.peek()
	return tk.kind == pyOp && tk.val == val
}

func (p *pyParser) peekOpAny(vals ...string) bool {
	for _, v := range vals {
		if p.peekOp(v) {
			return true
		}
	}
	return false
}

func (p *pyParser) expectIdent(name string) error {
	if !p.peekIdent(name) {
		return p.errorf("expected %s", name)
	}
	p.next()
	return nil
}

func (p *pyParser) expectOp(val string) error {
	if !p.peekOp(val) {
		return p.errorf("expected %s", val)
	}
	p.next()
	return nil
}

func (p *pyParser) atStmtEnd() bool {
	tk := p.peek()
	return tk.kind == pyNL || tk.kind == pyDedent || tk.kind == pyEOF || (tk.kind == pyOp && tk.val == ";")
}

func (p *pyParser) parseStmt() ([]pyStmt, error) {
	if p.peekOp("@") {
		s, err := p.parseDecorated()
		if err != nil {
			return nil, err
		}
		return []pyStmt{s}, nil
	}
	if p.peekIdent("async") || p.peekIdent("def") || p.peekIdent("class") || p.peekIdent("if") || p.peekIdent("elif") || p.peekIdent("while") || p.peekIdent("for") || p.peekIdent("try") || p.peekIdent("with") || p.looksLikeMatch() {
		s, err := p.parseCompound()
		if err != nil {
			return nil, err
		}
		return []pyStmt{s}, nil
	}
	return p.parseSimpleLine()
}

func (p *pyParser) parseCompound() (pyStmt, error) {
	switch {
	case p.peekIdent("async"):
		return p.parseAsync()
	case p.peekIdent("def"):
		return p.parseFunc()
	case p.peekIdent("class"):
		return p.parseClass()
	case p.peekIdent("if") || p.peekIdent("elif"):
		return p.parseIf()
	case p.peekIdent("while"):
		return p.parseWhile()
	case p.peekIdent("for"):
		return p.parseFor(false)
	case p.peekIdent("try"):
		return p.parseTry()
	case p.peekIdent("with"):
		return p.parseWith(false)
	case p.looksLikeMatch():
		return p.parseMatch()
	default:
		return pyStmt{}, p.errorf("expected compound statement")
	}
}

func (p *pyParser) parseAsync() (pyStmt, error) {
	if err := p.expectIdent("async"); err != nil {
		return pyStmt{}, err
	}
	switch {
	case p.peekIdent("def"):
		s, err := p.parseFunc()
		s.async = true
		return s, err
	case p.peekIdent("for"):
		return p.parseFor(true)
	case p.peekIdent("with"):
		return p.parseWith(true)
	default:
		return pyStmt{}, p.errorf("expected def, for, or with after async")
	}
}

func (p *pyParser) parseDecorated() (pyStmt, error) {
	for p.peekOp("@") {
		p.next()
		if _, err := p.parseExpr(); err != nil {
			return pyStmt{}, err
		}
		if p.peek().kind == pyNL {
			p.next()
		}
	}
	switch {
	case p.peekIdent("async"):
		return p.parseAsync()
	case p.peekIdent("def"):
		return p.parseFunc()
	case p.peekIdent("class"):
		return p.parseClass()
	default:
		return pyStmt{}, p.errorf("expected def or class after decorator")
	}
}

func (p *pyParser) parseFunc() (pyStmt, error) {
	if err := p.expectIdent("def"); err != nil {
		return pyStmt{}, err
	}
	if p.peek().kind != pyIdent {
		return pyStmt{}, p.errorf("expected function name")
	}
	name := p.next().val
	if err := p.consumeUntilColon(); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skFunc, name: name, body: body}, nil
}

func (p *pyParser) parseClass() (pyStmt, error) {
	if err := p.expectIdent("class"); err != nil {
		return pyStmt{}, err
	}
	if p.peek().kind != pyIdent {
		return pyStmt{}, p.errorf("expected class name")
	}
	name := p.next().val
	if err := p.consumeUntilColon(); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skClass, name: name, body: body}, nil
}

func (p *pyParser) consumeUntilColon() error {
	depth := 0
	for {
		tk := p.peek()
		if tk.kind == pyEOF {
			return p.errorf("expected ':'")
		}
		if tk.kind == pyOp {
			switch tk.val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			case ":":
				if depth == 0 {
					p.next()
					return nil
				}
			}
		}
		p.next()
	}
}

func (p *pyParser) parseSuite() ([]pyStmt, error) {
	if p.peek().kind == pyNL {
		p.next()
		if p.peek().kind != pyIndent {
			return nil, p.errorf("expected indented block")
		}
		p.next()
		var stmts []pyStmt
		for p.peek().kind != pyDedent && p.peek().kind != pyEOF {
			if p.peek().kind == pyNL {
				p.next()
				continue
			}
			part, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, part...)
		}
		if p.peek().kind == pyDedent {
			p.next()
		}
		if len(stmts) == 0 {
			return nil, p.errorf("empty block")
		}
		return stmts, nil
	}
	return p.parseSimpleLine()
}

func (p *pyParser) parseIf() (pyStmt, error) {
	p.next()
	test, err := p.parseExpr()
	if err != nil {
		return pyStmt{}, err
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	var orelse []pyStmt
	switch {
	case p.peekIdent("elif"):
		elif, err := p.parseIf()
		if err != nil {
			return pyStmt{}, err
		}
		orelse = []pyStmt{elif}
	case p.peekIdent("else"):
		p.next()
		if err := p.expectOp(":"); err != nil {
			return pyStmt{}, err
		}
		orelse, err = p.parseSuite()
		if err != nil {
			return pyStmt{}, err
		}
	}
	return pyStmt{kind: skIf, test: test, body: body, orelse: orelse}, nil
}

func (p *pyParser) parseWhile() (pyStmt, error) {
	p.next()
	test, err := p.parseExpr()
	if err != nil {
		return pyStmt{}, err
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	orelse, err := p.parseOptionalElse()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skWhile, test: test, body: body, orelse: orelse}, nil
}

func (p *pyParser) parseFor(async bool) (pyStmt, error) {
	if err := p.expectIdent("for"); err != nil {
		return pyStmt{}, err
	}
	if err := p.skipUntilIdent(0, "in"); err != nil {
		return pyStmt{}, err
	}
	if _, err := p.parseExpr(); err != nil {
		return pyStmt{}, err
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	orelse, err := p.parseOptionalElse()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skFor, async: async, body: body, orelse: orelse}, nil
}

func (p *pyParser) parseOptionalElse() ([]pyStmt, error) {
	if !p.peekIdent("else") {
		return nil, nil
	}
	p.next()
	if err := p.expectOp(":"); err != nil {
		return nil, err
	}
	return p.parseSuite()
}

func (p *pyParser) parseWith(async bool) (pyStmt, error) {
	if err := p.expectIdent("with"); err != nil {
		return pyStmt{}, err
	}
	var exprs []*px
	for {
		e, err := p.parseExpr()
		if err != nil {
			return pyStmt{}, err
		}
		exprs = append(exprs, e)
		if p.peekIdent("as") {
			p.next()
			if _, err := p.parseExpr(); err != nil {
				return pyStmt{}, err
			}
		}
		if !p.peekOp(",") {
			break
		}
		p.next()
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skWith, async: async, withExprs: exprs, body: body}, nil
}

func (p *pyParser) parseTry() (pyStmt, error) {
	if err := p.expectIdent("try"); err != nil {
		return pyStmt{}, err
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyStmt{}, err
	}
	var handlers []pyHandler
	for p.peekIdent("except") {
		h, err := p.parseExcept()
		if err != nil {
			return pyStmt{}, err
		}
		handlers = append(handlers, h)
	}
	var orelse []pyStmt
	if p.peekIdent("else") {
		p.next()
		if err := p.expectOp(":"); err != nil {
			return pyStmt{}, err
		}
		orelse, err = p.parseSuite()
		if err != nil {
			return pyStmt{}, err
		}
	}
	var final []pyStmt
	if p.peekIdent("finally") {
		p.next()
		if err := p.expectOp(":"); err != nil {
			return pyStmt{}, err
		}
		final, err = p.parseSuite()
		if err != nil {
			return pyStmt{}, err
		}
	}
	if len(handlers) == 0 && len(final) == 0 {
		return pyStmt{}, p.errorf("try without except or finally")
	}
	return pyStmt{kind: skTry, body: body, orelse: orelse, final: final, handlers: handlers}, nil
}

func (p *pyParser) parseExcept() (pyHandler, error) {
	if err := p.expectIdent("except"); err != nil {
		return pyHandler{}, err
	}
	if p.peekOp("*") {
		p.next()
	}
	if p.peekOp(":") {
		p.next()
		body, err := p.parseSuite()
		if err != nil {
			return pyHandler{}, err
		}
		return pyHandler{names: []string{"<bare-except>"}, body: body}, nil
	}
	var types []*px
	for {
		e, err := p.parseExpr()
		if err != nil {
			return pyHandler{}, err
		}
		types = append(types, e)
		if !p.peekOp(",") {
			break
		}
		// Python 3 rejects `except A, B:` (PEP 758 needs parentheses). CPython's
		// message is what the CHECK-01 CLI reports for C-05.
		line := p.peek().line
		return pyHandler{}, fmt.Errorf("multiple exception types must be parenthesized (<unknown>, line %d)", line)
	}
	if p.peekIdent("as") {
		p.next()
		if _, err := p.parseExpr(); err != nil {
			return pyHandler{}, err
		}
	}
	if err := p.expectOp(":"); err != nil {
		return pyHandler{}, err
	}
	body, err := p.parseSuite()
	if err != nil {
		return pyHandler{}, err
	}
	return pyHandler{names: namesFromExcept(types), body: body}, nil
}

func namesFromExcept(exprs []*px) []string {
	var raw []string
	for _, e := range exprs {
		if e != nil && e.op == "tuple" {
			for _, k := range e.kids {
				raw = append(raw, unparse(k))
			}
			continue
		}
		raw = append(raw, unparse(e))
	}
	if len(raw) == 0 {
		return []string{"<bare-except>"}
	}
	return uniqSorted(raw)
}

func (p *pyParser) looksLikeMatch() bool {
	if !p.peekIdent("match") {
		return false
	}
	depth := 0
	for i := p.pos + 1; i < len(p.toks); i++ {
		tk := p.toks[i]
		if tk.kind == pyNL || tk.kind == pyDedent || tk.kind == pyEOF {
			return false
		}
		if tk.kind != pyOp {
			continue
		}
		switch tk.val {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case "=", "+=", "-=", "*=", "/=", "%=", "//=", "**=", "@=", "&=", "|=", "^=", ">>=", "<<=", ":=":
			if depth == 0 {
				return false
			}
		case ":":
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func (p *pyParser) parseMatch() (pyStmt, error) {
	if err := p.expectIdent("match"); err != nil {
		return pyStmt{}, err
	}
	if _, err := p.parseExpr(); err != nil {
		return pyStmt{}, err
	}
	if err := p.expectOp(":"); err != nil {
		return pyStmt{}, err
	}
	if p.peek().kind != pyNL {
		return pyStmt{}, p.errorf("expected newline after match")
	}
	p.next()
	if p.peek().kind != pyIndent {
		return pyStmt{}, p.errorf("expected match cases")
	}
	p.next()
	var cases []pyCase
	for p.peekIdent("case") {
		p.next()
		depth := 0
		for {
			if p.peek().kind == pyEOF {
				return pyStmt{}, p.errorf("unterminated case")
			}
			if p.peekOpAny("(", "[", "{") {
				depth++
				p.next()
				continue
			}
			if p.peekOpAny(")", "]", "}") {
				if depth > 0 {
					depth--
				}
				p.next()
				continue
			}
			if depth == 0 && (p.peekIdent("if") || p.peekOp(":")) {
				break
			}
			p.next()
		}
		var guard *px
		if p.peekIdent("if") {
			p.next()
			g, err := p.parseExpr()
			if err != nil {
				return pyStmt{}, err
			}
			guard = g
		}
		if err := p.expectOp(":"); err != nil {
			return pyStmt{}, err
		}
		body, err := p.parseSuite()
		if err != nil {
			return pyStmt{}, err
		}
		cases = append(cases, pyCase{guard: guard, body: body})
	}
	if p.peek().kind == pyDedent {
		p.next()
	}
	if len(cases) == 0 {
		return pyStmt{}, p.errorf("match without case")
	}
	return pyStmt{kind: skMatch, cases: cases}, nil
}

func (p *pyParser) parseSimpleLine() ([]pyStmt, error) {
	var out []pyStmt
	for {
		s, err := p.parseSmall()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		if !p.peekOp(";") {
			break
		}
		p.next()
		if p.atStmtEnd() {
			break
		}
	}
	if p.peek().kind == pyNL {
		p.next()
	}
	return out, nil
}

func (p *pyParser) parseSmall() (pyStmt, error) {
	switch {
	case p.peekIdent("pass"), p.peekIdent("break"), p.peekIdent("continue"):
		p.next()
		return pyStmt{kind: skOther}, nil
	case p.peekIdent("import"), p.peekIdent("from"), p.peekIdent("global"), p.peekIdent("nonlocal"), p.peekIdent("del"):
		if err := p.skipRest(); err != nil {
			return pyStmt{}, err
		}
		return pyStmt{kind: skOther}, nil
	case p.peekIdent("assert"):
		return p.parseAssert()
	case p.peekIdent("return"):
		return p.parseReturn()
	case p.peekIdent("raise"):
		return p.parseRaise()
	}
	e, err := p.parseExprList()
	if err != nil {
		return pyStmt{}, err
	}
	if p.peekOp(":") && (e == nil || e.op != "tuple") {
		p.next()
		ann, err := p.parseExpr()
		if err != nil {
			return pyStmt{}, err
		}
		var val *px
		if p.peekOp("=") {
			p.next()
			val, err = p.parseExprList()
			if err != nil {
				return pyStmt{}, err
			}
		}
		return pyStmt{kind: skAnn, value: val, ann: ann}, nil
	}
	if p.peek().kind == pyOp && isAug(p.peek().val) {
		p.next()
		val, err := p.parseExprList()
		if err != nil {
			return pyStmt{}, err
		}
		return pyStmt{kind: skAug, value: val}, nil
	}
	if p.peekOp("=") {
		var val *px
		for p.peekOp("=") {
			p.next()
			val, err = p.parseExprList()
			if err != nil {
				return pyStmt{}, err
			}
		}
		return pyStmt{kind: skAssign, value: val}, nil
	}
	return pyStmt{kind: skExpr, value: e}, nil
}

func isAug(v string) bool {
	switch v {
	case "+=", "-=", "*=", "/=", "%=", "//=", "**=", "@=", "&=", "|=", "^=", ">>=", "<<=":
		return true
	default:
		return false
	}
}

func (p *pyParser) parseAssert() (pyStmt, error) {
	p.next()
	test, err := p.parseExpr()
	if err != nil {
		return pyStmt{}, err
	}
	var msg *px
	if p.peekOp(",") {
		p.next()
		msg, err = p.parseExpr()
		if err != nil {
			return pyStmt{}, err
		}
	}
	return pyStmt{kind: skAssert, test: test, value: msg}, nil
}

func (p *pyParser) parseReturn() (pyStmt, error) {
	p.next()
	if p.atStmtEnd() {
		return pyStmt{kind: skReturn}, nil
	}
	e, err := p.parseExprList()
	if err != nil {
		return pyStmt{}, err
	}
	return pyStmt{kind: skReturn, value: e}, nil
}

func (p *pyParser) parseRaise() (pyStmt, error) {
	p.next()
	if p.atStmtEnd() {
		return pyStmt{kind: skRaise}, nil
	}
	if _, err := p.parseExpr(); err != nil {
		return pyStmt{}, err
	}
	if p.peekIdent("from") {
		p.next()
		if _, err := p.parseExpr(); err != nil {
			return pyStmt{}, err
		}
	}
	return pyStmt{kind: skRaise}, nil
}

func (p *pyParser) skipRest() error {
	depth := 0
	for p.peek().kind != pyEOF {
		if depth == 0 && p.atStmtEnd() {
			return nil
		}
		if p.peek().kind == pyOp {
			switch p.peek().val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			}
		}
		prev := p.pos
		p.next()
		if p.pos == prev {
			return p.errorf("stuck while skipping statement")
		}
	}
	return nil
}

func (p *pyParser) skipUntilIdent(depth int, name string) error {
	for {
		if p.peek().kind == pyEOF {
			return p.errorf("expected %s", name)
		}
		if depth == 0 && p.peekIdent(name) {
			p.next()
			return nil
		}
		if p.peek().kind == pyOp {
			switch p.peek().val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			}
		}
		p.next()
	}
}

func (p *pyParser) parseExprList() (*px, error) {
	e, err := p.parseStarItem()
	if err != nil {
		return nil, err
	}
	if !p.peekOp(",") {
		return e, nil
	}
	kids := []*px{e}
	for p.peekOp(",") {
		p.next()
		if p.atStmtEnd() || p.peekOp(")") || p.peekOp("]") || p.peekOp("}") || p.peekOp(":") || p.peekOp("=") || p.peekIdent("as") {
			break
		}
		elt, err := p.parseStarItem()
		if err != nil {
			return nil, err
		}
		kids = append(kids, elt)
	}
	return &px{op: "tuple", kids: kids}, nil
}

func (p *pyParser) parseStarItem() (*px, error) {
	if p.peekIdent("yield") {
		return p.parseYield()
	}
	if p.peekOp("*") {
		p.next()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		return &px{op: "star", lit: "*", kids: []*px{e}}, nil
	}
	return p.parseNamed()
}

func (p *pyParser) parseExpr() (*px, error) {
	if p.peekIdent("yield") {
		return p.parseYield()
	}
	return p.parseNamed()
}

func (p *pyParser) parseYield() (*px, error) {
	p.next()
	if p.peekIdent("from") {
		p.next()
		e, err := p.parseNamed()
		if err != nil {
			return nil, err
		}
		return &px{op: "yield", kids: []*px{e}}, nil
	}
	if p.exprEnd() {
		return &px{op: "yield"}, nil
	}
	e, err := p.parseNamed()
	if err != nil {
		return nil, err
	}
	kids := []*px{e}
	for p.peekOp(",") {
		p.next()
		if p.exprEnd() {
			break
		}
		elt, err := p.parseNamed()
		if err != nil {
			return nil, err
		}
		kids = append(kids, elt)
	}
	if len(kids) == 1 {
		return &px{op: "yield", kids: kids}, nil
	}
	return &px{op: "yield", kids: []*px{{op: "tuple", kids: kids}}}, nil
}

func (p *pyParser) exprEnd() bool {
	tk := p.peek()
	if tk.kind == pyNL || tk.kind == pyDedent || tk.kind == pyEOF {
		return true
	}
	if tk.kind == pyOp {
		switch tk.val {
		case ";", ")", "]", "}", ":", "=", "+=", "-=", "*=", "/=", "%=", "//=", "**=", "@=", "&=", "|=", "^=", ">>=", "<<=":
			return true
		}
	}
	if tk.kind == pyIdent && (tk.val == "as" || tk.val == "else" || tk.val == "elif" || tk.val == "from") {
		return true
	}
	return false
}

func (p *pyParser) parseNamed() (*px, error) {
	e, err := p.parseIfExpr()
	if err != nil {
		return nil, err
	}
	if p.peekOp(":=") {
		p.next()
		rhs, err := p.parseIfExpr()
		if err != nil {
			return nil, err
		}
		return &px{op: "named", kids: []*px{e, rhs}}, nil
	}
	return e, nil
}

func (p *pyParser) parseIfExpr() (*px, error) {
	body, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.peekIdent("if") {
		return body, nil
	}
	p.next()
	test, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.peekIdent("else") {
		return nil, p.errorf("expected else in conditional expression")
	}
	p.next()
	orelse, err := p.parseIfExpr()
	if err != nil {
		return nil, err
	}
	return &px{op: "ifexp", kids: []*px{test, body, orelse}}, nil
}

func (p *pyParser) parseOr() (*px, error)  { return p.parseBool("or", p.parseAnd) }
func (p *pyParser) parseAnd() (*px, error) { return p.parseBool("and", p.parseNot) }

func (p *pyParser) parseBool(op string, next func() (*px, error)) (*px, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	if !p.peekIdent(op) {
		return left, nil
	}
	kids := []*px{left}
	for p.peekIdent(op) {
		p.next()
		right, err := next()
		if err != nil {
			return nil, err
		}
		kids = append(kids, right)
	}
	return &px{op: op, kids: kids}, nil
}

func (p *pyParser) parseNot() (*px, error) {
	if p.peekIdent("not") && !(p.peekN(1).kind == pyIdent && p.peekN(1).val == "in") {
		p.next()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &px{op: "not", kids: []*px{operand}}, nil
	}
	return p.parseComparison()
}

func (p *pyParser) parseComparison() (*px, error) {
	left, err := p.parseBitOr()
	if err != nil {
		return nil, err
	}
	var ops []string
	var comps []*px
	for {
		op, ok := p.matchCompOp()
		if !ok {
			break
		}
		right, err := p.parseBitOr()
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
		comps = append(comps, right)
	}
	if len(ops) == 0 {
		return left, nil
	}
	return &px{op: "compare", lit: strings.Join(ops, ","), kids: append([]*px{left}, comps...)}, nil
}

func (p *pyParser) matchCompOp() (string, bool) {
	if p.peekIdent("not") && p.peekN(1).kind == pyIdent && p.peekN(1).val == "in" {
		p.next()
		p.next()
		return "not in", true
	}
	if p.peekIdent("is") && p.peekN(1).kind == pyIdent && p.peekN(1).val == "not" {
		p.next()
		p.next()
		return "is not", true
	}
	if p.peekIdent("is") {
		p.next()
		return "is", true
	}
	if p.peekIdent("in") {
		p.next()
		return "in", true
	}
	switch p.peek().val {
	case "==", "!=", "<", "<=", ">", ">=":
		if p.peek().kind == pyOp {
			op := p.next().val
			return op, true
		}
	}
	return "", false
}

func (p *pyParser) parseBitOr() (*px, error) {
	return p.parseBin([]string{"|"}, p.parseBitXor)
}
func (p *pyParser) parseBitXor() (*px, error) {
	return p.parseBin([]string{"^"}, p.parseBitAnd)
}
func (p *pyParser) parseBitAnd() (*px, error) {
	return p.parseBin([]string{"&"}, p.parseShift)
}
func (p *pyParser) parseShift() (*px, error) {
	return p.parseBin([]string{"<<", ">>"}, p.parseSum)
}
func (p *pyParser) parseSum() (*px, error) {
	return p.parseBin([]string{"+", "-"}, p.parseTerm)
}
func (p *pyParser) parseTerm() (*px, error) {
	return p.parseBin([]string{"*", "/", "//", "%", "@"}, p.parseFactor)
}

func (p *pyParser) parseBin(ops []string, next func() (*px, error)) (*px, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for p.peekOpAny(ops...) {
		op := p.next().val
		right, err := next()
		if err != nil {
			return nil, err
		}
		left = &px{op: "bin", lit: op, kids: []*px{left, right}}
	}
	return left, nil
}

func (p *pyParser) parseFactor() (*px, error) {
	if p.peekOp("+") || p.peekOp("-") || p.peekOp("~") {
		op := p.next().val
		operand, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return &px{op: "unary", lit: op, kids: []*px{operand}}, nil
	}
	return p.parsePower()
}

func (p *pyParser) parsePower() (*px, error) {
	base, err := p.parseAwaitPrimary()
	if err != nil {
		return nil, err
	}
	if p.peekOp("**") {
		p.next()
		exp, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return &px{op: "bin", lit: "**", kids: []*px{base, exp}}, nil
	}
	return base, nil
}

func (p *pyParser) parseAwaitPrimary() (*px, error) {
	if p.peekIdent("await") {
		p.next()
		inner, err := p.parseAwaitPrimary()
		if err != nil {
			return nil, err
		}
		return &px{op: "await", kids: []*px{inner}}, nil
	}
	return p.parsePrimary()
}

func (p *pyParser) parsePrimary() (*px, error) {
	atom, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.peekOp("("):
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			atom = &px{op: "call", kids: append([]*px{atom}, args...)}
		case p.peekOp("["):
			p.next()
			inner, err := p.parseSubContents()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("]"); err != nil {
				return nil, err
			}
			atom = &px{op: "sub", kids: append([]*px{atom}, inner...)}
		case p.peekOp("."):
			p.next()
			if p.peek().kind != pyIdent {
				return nil, p.errorf("expected attribute name")
			}
			name := p.next().val
			atom = &px{op: "attr", lit: name, kids: []*px{atom}}
		default:
			return atom, nil
		}
	}
}

func (p *pyParser) parseAtom() (*px, error) {
	tk := p.peek()
	switch tk.kind {
	case pyIdent:
		if tk.val == "lambda" {
			return p.parseLambda()
		}
		p.next()
		return &px{op: "name", lit: tk.val}, nil
	case pyNum:
		p.next()
		return &px{op: "num", lit: tk.val}, nil
	case pyStr:
		return p.parseStringAtom()
	case pyOp:
		switch tk.val {
		case "(":
			return p.parseParen()
		case "[":
			return p.parseList()
		case "{":
			return p.parseBrace()
		case "...":
			p.next()
			return &px{op: "name", lit: "..."}, nil
		}
	}
	return nil, p.errorf("expected expression")
}

func (p *pyParser) parseStringAtom() (*px, error) {
	tok := p.next()
	raw := tok.val
	exprs := append([]string{}, tok.exprs...)
	for p.peek().kind == pyStr && len(p.peek().exprs) == 0 && len(exprs) == 0 {
		raw += p.next().val
	}
	if len(exprs) == 0 {
		return &px{op: "str", lit: raw}, nil
	}
	var kids []*px
	for _, ex := range exprs {
		tree, err := parsePythonExpr(ex)
		if err != nil {
			return nil, err
		}
		kids = append(kids, tree)
	}
	return &px{op: "joined", lit: raw, kids: kids}, nil
}

func parsePythonExpr(src string) (*px, error) {
	toks, err := tokenizePy(src)
	if err != nil {
		return nil, err
	}
	p := &pyParser{toks: toks}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (p *pyParser) parseLambda() (*px, error) {
	p.next()
	depth := 0
	for {
		tk := p.peek()
		if tk.kind == pyEOF {
			return nil, p.errorf("unterminated lambda")
		}
		if tk.kind == pyOp {
			switch tk.val {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			case ":":
				if depth == 0 {
					p.next()
					if _, err := p.parseIfExpr(); err != nil {
						return nil, err
					}
					return &px{op: "lambda"}, nil
				}
			}
		}
		p.next()
	}
}

func (p *pyParser) parseParen() (*px, error) {
	p.next()
	if p.peekOp(")") {
		p.next()
		return &px{op: "tuple"}, nil
	}
	if p.peekIdent("yield") {
		y, err := p.parseYield()
		if err != nil {
			return nil, err
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return y, nil
	}
	e, err := p.parseStarItem()
	if err != nil {
		return nil, err
	}
	if p.isCompFor() {
		kids := []*px{e}
		more, err := p.consumeComp()
		if err != nil {
			return nil, err
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return &px{op: "comp", kids: append(kids, more...)}, nil
	}
	if p.peekOp(",") {
		kids := []*px{e}
		for p.peekOp(",") {
			p.next()
			if p.peekOp(")") {
				break
			}
			elt, err := p.parseStarItem()
			if err != nil {
				return nil, err
			}
			kids = append(kids, elt)
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return &px{op: "tuple", kids: kids}, nil
	}
	if err := p.expectOp(")"); err != nil {
		return nil, err
	}
	return e, nil
}

func (p *pyParser) parseList() (*px, error) {
	p.next()
	if p.peekOp("]") {
		p.next()
		return &px{op: "list"}, nil
	}
	e, err := p.parseStarItem()
	if err != nil {
		return nil, err
	}
	if p.isCompFor() {
		more, err := p.consumeComp()
		if err != nil {
			return nil, err
		}
		if err := p.expectOp("]"); err != nil {
			return nil, err
		}
		return &px{op: "comp", kids: append([]*px{e}, more...)}, nil
	}
	kids := []*px{e}
	for p.peekOp(",") {
		p.next()
		if p.peekOp("]") {
			break
		}
		elt, err := p.parseStarItem()
		if err != nil {
			return nil, err
		}
		kids = append(kids, elt)
	}
	if err := p.expectOp("]"); err != nil {
		return nil, err
	}
	return &px{op: "list", kids: kids}, nil
}

func (p *pyParser) parseBrace() (*px, error) {
	p.next()
	if p.peekOp("}") {
		p.next()
		return &px{op: "dict"}, nil
	}
	if p.peekOp("**") {
		p.next()
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		kids := []*px{e}
		for p.peekOp(",") {
			p.next()
			if p.peekOp("}") {
				break
			}
			if p.peekOp("**") {
				p.next()
			}
			elt, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			kids = append(kids, elt)
			if p.peekOp(":") {
				p.next()
				v, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				kids = append(kids, v)
			}
		}
		if err := p.expectOp("}"); err != nil {
			return nil, err
		}
		return &px{op: "dict", kids: kids}, nil
	}
	e, err := p.parseStarItem()
	if err != nil {
		return nil, err
	}
	if p.peekOp(":") {
		p.next()
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		kids := []*px{e, v}
		if p.isCompFor() {
			more, err := p.consumeComp()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("}"); err != nil {
				return nil, err
			}
			return &px{op: "comp", kids: append(kids, more...)}, nil
		}
		for p.peekOp(",") {
			p.next()
			if p.peekOp("}") {
				break
			}
			k, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(":"); err != nil {
				return nil, err
			}
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			kids = append(kids, k, val)
		}
		if err := p.expectOp("}"); err != nil {
			return nil, err
		}
		return &px{op: "dict", kids: kids}, nil
	}
	if p.isCompFor() {
		more, err := p.consumeComp()
		if err != nil {
			return nil, err
		}
		if err := p.expectOp("}"); err != nil {
			return nil, err
		}
		return &px{op: "comp", kids: append([]*px{e}, more...)}, nil
	}
	kids := []*px{e}
	for p.peekOp(",") {
		p.next()
		if p.peekOp("}") {
			break
		}
		elt, err := p.parseStarItem()
		if err != nil {
			return nil, err
		}
		kids = append(kids, elt)
	}
	if err := p.expectOp("}"); err != nil {
		return nil, err
	}
	return &px{op: "set", kids: kids}, nil
}

func (p *pyParser) isCompFor() bool {
	if p.peekIdent("for") {
		return true
	}
	return p.peekIdent("async") && p.peekN(1).kind == pyIdent && p.peekN(1).val == "for"
}

func (p *pyParser) consumeComp() ([]*px, error) {
	var kids []*px
	for p.isCompFor() {
		if p.peekIdent("async") {
			p.next()
		}
		if err := p.expectIdent("for"); err != nil {
			return nil, err
		}
		if err := p.skipUntilIdent(0, "in"); err != nil {
			return nil, err
		}
		iter, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		kids = append(kids, iter)
		for p.peekIdent("if") {
			p.next()
			filt, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			kids = append(kids, filt)
		}
	}
	return kids, nil
}

func (p *pyParser) parseArgList() ([]*px, error) {
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	var args []*px
	if p.peekOp(")") {
		p.next()
		return args, nil
	}
	for {
		if p.peekOp("*") || p.peekOp("**") {
			op := p.next().val
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, &px{op: "star", lit: op, kids: []*px{e}})
		} else if p.peek().kind == pyIdent && p.peekN(1).kind == pyOp && p.peekN(1).val == "=" {
			name := p.next().val
			p.next()
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, &px{op: "kw", lit: name, kids: []*px{e}})
		} else {
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.isCompFor() && len(args) == 0 {
				more, err := p.consumeComp()
				if err != nil {
					return nil, err
				}
				e = &px{op: "comp", kids: append([]*px{e}, more...)}
			}
			args = append(args, e)
		}
		if p.peekOp(",") {
			p.next()
			if p.peekOp(")") {
				break
			}
			continue
		}
		break
	}
	if err := p.expectOp(")"); err != nil {
		return nil, err
	}
	return args, nil
}

func (p *pyParser) parseSubContents() ([]*px, error) {
	var kids []*px
	if p.peekOp("]") {
		return kids, nil
	}
	for {
		part, err := p.parseSlice()
		if err != nil {
			return nil, err
		}
		kids = append(kids, part...)
		if !p.peekOp(",") {
			break
		}
		p.next()
		if p.peekOp("]") {
			break
		}
	}
	return kids, nil
}

func (p *pyParser) parseSlice() ([]*px, error) {
	var kids []*px
	if !p.peekOp(":") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		kids = append(kids, e)
		if !p.peekOp(":") {
			return kids, nil
		}
	}
	if err := p.expectOp(":"); err != nil {
		return nil, err
	}
	if !p.peekOp(":") && !p.peekOp(",") && !p.peekOp("]") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		kids = append(kids, e)
	}
	if p.peekOp(":") {
		p.next()
		if !p.peekOp(",") && !p.peekOp("]") {
			e, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			kids = append(kids, e)
		}
	}
	return kids, nil
}

func unparse(e *px) string {
	if e == nil {
		return ""
	}
	switch e.op {
	case "name", "num", "str":
		return e.lit
	case "attr":
		if len(e.kids) == 0 {
			return e.lit
		}
		return unparse(e.kids[0]) + "." + e.lit
	case "call":
		if len(e.kids) == 0 {
			return "()"
		}
		args := make([]string, 0, len(e.kids)-1)
		for _, k := range e.kids[1:] {
			args = append(args, unparse(k))
		}
		return unparse(e.kids[0]) + "(" + strings.Join(args, ", ") + ")"
	case "kw":
		if len(e.kids) == 0 {
			return e.lit
		}
		return e.lit + "=" + unparse(e.kids[0])
	case "star":
		if len(e.kids) == 0 {
			return e.lit
		}
		return e.lit + unparse(e.kids[0])
	case "tuple":
		parts := make([]string, len(e.kids))
		for i, k := range e.kids {
			parts[i] = unparse(k)
		}
		if len(parts) == 1 {
			return "(" + parts[0] + ",)"
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case "list":
		parts := make([]string, len(e.kids))
		for i, k := range e.kids {
			parts[i] = unparse(k)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case "await":
		if len(e.kids) == 0 {
			return "await"
		}
		return "await " + unparse(e.kids[0])
	case "ifexp":
		if len(e.kids) < 3 {
			return "ifexp"
		}
		return unparse(e.kids[1]) + " if " + unparse(e.kids[0]) + " else " + unparse(e.kids[2])
	case "compare":
		if len(e.kids) == 0 {
			return e.lit
		}
		ops := strings.Split(e.lit, ",")
		var b strings.Builder
		b.WriteString(unparse(e.kids[0]))
		for i, op := range ops {
			if i+1 < len(e.kids) {
				b.WriteString(" ")
				b.WriteString(op)
				b.WriteString(" ")
				b.WriteString(unparse(e.kids[i+1]))
			}
		}
		return b.String()
	case "not":
		if len(e.kids) == 0 {
			return "not"
		}
		return "not " + unparse(e.kids[0])
	case "and", "or":
		parts := make([]string, len(e.kids))
		for i, k := range e.kids {
			parts[i] = unparse(k)
		}
		return strings.Join(parts, " "+e.op+" ")
	case "unary":
		if len(e.kids) == 0 {
			return e.lit
		}
		return e.lit + unparse(e.kids[0])
	case "bin":
		if len(e.kids) < 2 {
			return e.lit
		}
		return unparse(e.kids[0]) + " " + e.lit + " " + unparse(e.kids[1])
	case "sub":
		if len(e.kids) == 0 {
			return "[]"
		}
		inner := make([]string, 0, len(e.kids)-1)
		for _, k := range e.kids[1:] {
			inner = append(inner, unparse(k))
		}
		return unparse(e.kids[0]) + "[" + strings.Join(inner, ":") + "]"
	case "yield":
		if len(e.kids) == 0 {
			return "yield"
		}
		return "yield " + unparse(e.kids[0])
	case "lambda":
		return "lambda"
	case "joined":
		return e.lit
	default:
		if e.lit != "" && len(e.kids) == 0 {
			return e.lit
		}
		parts := make([]string, len(e.kids))
		for i, k := range e.kids {
			parts[i] = unparse(k)
		}
		if e.lit != "" {
			return e.op + ":" + e.lit + "(" + strings.Join(parts, ", ") + ")"
		}
		return e.op + "(" + strings.Join(parts, ", ") + ")"
	}
}

type pyFacts struct {
	ifCount     int
	ifElse      int
	tryElse     int
	loopElse    int
	retCount    int
	contexts    []string
	finallyHash []string
	exceptTypes [][][]string
	cond        map[string]int
	exceptRaise int
}

type pyColl struct {
	pyFacts
	exceptDepth int
}

func screenPython(beforeSrc, afterSrc, logical string) ([]Candidate, error) {
	beforeStmts, err := parsePython(beforeSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Python file %s (before): %s", logical, err.Error())
	}
	afterStmts, err := parsePython(afterSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Python file %s (after): %s", logical, err.Error())
	}
	beforeF := extractPyFuncs(beforeStmts)
	afterF := extractPyFuncs(afterStmts)
	var out []Candidate
	var removed []string
	for name := range beforeF {
		if _, ok := afterF[name]; !ok {
			removed = append(removed, name)
		}
	}
	sortStrings(removed)
	for _, name := range removed {
		out = append(out, makeCandidate(logical, name, "python.function_removed", "defined ("+name+")", "<removed>"))
	}
	var common []string
	for name := range beforeF {
		if _, ok := afterF[name]; ok {
			common = append(common, name)
		}
	}
	sortStrings(common)
	for _, name := range common {
		out = append(out, comparePyFunc(logical, name, beforeF[name], afterF[name])...)
	}
	return out, nil
}

func extractPyFuncs(stmts []pyStmt) map[string]pyFacts {
	out := map[string]pyFacts{}
	var walk func([]pyStmt, string)
	walk = func(list []pyStmt, prefix string) {
		for _, s := range list {
			switch s.kind {
			case skClass:
				name := s.name
				if prefix != "" {
					name = prefix + "." + s.name
				}
				walk(s.body, name)
			case skFunc:
				name := s.name
				if prefix != "" {
					name = prefix + "." + s.name
				}
				out[name] = analyzePy(s.body)
				walk(s.body, name)
			}
		}
	}
	walk(stmts, "")
	return out
}

func analyzePy(body []pyStmt) pyFacts {
	c := &pyColl{pyFacts: pyFacts{cond: map[string]int{}}}
	c.walk(body)
	c.contexts = append([]string(nil), c.contexts...)
	sortStrings(c.contexts)
	return c.pyFacts
}

func (c *pyColl) walk(stmts []pyStmt) {
	for _, s := range stmts {
		c.stmt(s)
	}
}

func (c *pyColl) stmt(s pyStmt) {
	switch s.kind {
	case skFunc, skClass:
		return
	case skIf:
		c.ifCount++
		if len(s.orelse) > 0 {
			c.ifElse++
		}
		collectCond(s.test, c.cond)
		c.walk(s.body)
		c.walk(s.orelse)
	case skTry:
		if len(s.orelse) > 0 {
			c.tryElse++
		}
		if len(s.final) > 0 {
			c.finallyHash = append(c.finallyHash, hashStmts(s.final))
		}
		var handlers [][]string
		for _, h := range s.handlers {
			handlers = append(handlers, h.names)
		}
		c.exceptTypes = append(c.exceptTypes, handlers)
		for _, h := range s.handlers {
			c.exceptDepth++
			c.walk(h.body)
			c.exceptDepth--
		}
		c.walk(s.body)
		c.walk(s.orelse)
		c.walk(s.final)
	case skFor, skWhile:
		if s.kind == skWhile {
			collectCond(s.test, c.cond)
		}
		if len(s.orelse) > 0 {
			c.loopElse++
		}
		c.walk(s.body)
		c.walk(s.orelse)
	case skWith:
		prefix := "with"
		if s.async {
			prefix = "async with"
		}
		for _, e := range s.withExprs {
			c.contexts = append(c.contexts, prefix+":"+unparse(e))
		}
		c.walk(s.body)
	case skMatch:
		for _, cs := range s.cases {
			if cs.guard != nil {
				collectCond(cs.guard, c.cond)
			}
			c.walk(cs.body)
		}
	case skReturn:
		c.retCount++
		collectIf(s.value, c.cond)
	case skRaise:
		if c.exceptDepth > 0 {
			c.exceptRaise++
		}
	case skAssign, skAnn, skAug, skExpr:
		collectIf(s.value, c.cond)
	}
}

func collectCond(e *px, counts map[string]int) {
	if e == nil {
		return
	}
	switch e.op {
	case "or", "and":
		n := len(e.kids) - 1
		if n < 1 {
			n = 1
		}
		counts[e.op] += n
		for _, k := range e.kids {
			collectCond(k, counts)
		}
	case "not":
		counts["not"]++
		if len(e.kids) == 1 {
			collectCond(e.kids[0], counts)
		}
	case "unary":
		if len(e.kids) == 1 {
			collectCond(e.kids[0], counts)
		}
	case "compare":
		for _, op := range strings.Split(e.lit, ",") {
			if op != "" {
				counts[op]++
			}
		}
		for _, k := range e.kids {
			collectCond(k, counts)
		}
	case "call":
		for i, k := range e.kids {
			if i == 0 {
				continue
			}
			collectCond(k, counts)
		}
	case "ifexp":
		for _, k := range e.kids {
			collectCond(k, counts)
		}
	}
}

func collectIf(e *px, counts map[string]int) {
	if e == nil || e.op == "lambda" {
		return
	}
	if e.op == "ifexp" {
		if len(e.kids) > 0 {
			collectCond(e.kids[0], counts)
		}
		if len(e.kids) > 1 {
			collectIf(e.kids[1], counts)
		}
		if len(e.kids) > 2 {
			collectIf(e.kids[2], counts)
		}
		return
	}
	for _, k := range e.kids {
		collectIf(k, counts)
	}
}

func hashStmts(stmts []pyStmt) string {
	parts := make([]string, 0, len(stmts))
	for _, s := range stmts {
		if isDocstring(s) {
			continue
		}
		parts = append(parts, astDumpStmt(s))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

func astDumpStmt(s pyStmt) string {
	switch s.kind {
	case skExpr:
		return "Expr(value=" + astDumpExpr(s.value) + ")"
	case skReturn:
		if s.value == nil {
			return "Return()"
		}
		return "Return(value=" + astDumpExpr(s.value) + ")"
	case skRaise:
		if s.value == nil && s.test == nil {
			return "Raise()"
		}
		if s.test == nil {
			return "Raise(exc=" + astDumpExpr(s.value) + ")"
		}
		return "Raise(exc=" + astDumpExpr(s.value) + ", cause=" + astDumpExpr(s.test) + ")"
	case skAssert:
		if s.value == nil {
			return "Assert(test=" + astDumpExpr(s.test) + ")"
		}
		return "Assert(test=" + astDumpExpr(s.test) + ", msg=" + astDumpExpr(s.value) + ")"
	default:
		var b strings.Builder
		dumpStmt(&b, s)
		return strings.TrimRight(b.String(), " \n")
	}
}

func astDumpExpr(e *px) string {
	if e == nil {
		return "None"
	}
	switch e.op {
	case "name":
		switch e.lit {
		case "None":
			return "Constant(value=None)"
		case "True":
			return "Constant(value=True)"
		case "False":
			return "Constant(value=False)"
		case "...":
			return "Constant(value=Ellipsis)"
		default:
			return "Name(id=" + pyQuote(e.lit) + ", ctx=Load())"
		}
	case "num":
		return "Constant(value=" + strings.ReplaceAll(e.lit, "_", "") + ")"
	case "str":
		lit := e.lit
		if len(lit) >= 2 && (lit[0] == '\'' || lit[0] == '"') {
			return "Constant(value=" + lit + ")"
		}
		return "Constant(value=" + pyQuote(lit) + ")"
	case "attr":
		return "Attribute(value=" + astDumpExpr(kid(e, 0)) + ", attr=" + pyQuote(e.lit) + ", ctx=Load())"
	case "await":
		return "Await(value=" + astDumpExpr(kid(e, 0)) + ")"
	case "call":
		var args, kws []string
		for _, k := range e.kids[1:] {
			if k != nil && k.op == "kw" {
				if k.lit == "" || k.lit == "**" {
					kws = append(kws, "keyword(value="+astDumpExpr(kid(k, 0))+")")
				} else {
					kws = append(kws, "keyword(arg="+pyQuote(strings.TrimSuffix(k.lit, "="))+", value="+astDumpExpr(kid(k, 0))+")")
				}
				continue
			}
			if k != nil && k.op == "star" {
				args = append(args, "Starred(value="+astDumpExpr(kid(k, 0))+", ctx=Load())")
				continue
			}
			args = append(args, astDumpExpr(k))
		}
		fn := "None"
		if len(e.kids) > 0 {
			fn = astDumpExpr(e.kids[0])
		}
		return "Call(func=" + fn + ", args=[" + strings.Join(args, ", ") + "], keywords=[" + strings.Join(kws, ", ") + "])"
	case "tuple":
		return "Tuple(elts=[" + astDumpList(e.kids) + "], ctx=Load())"
	case "list":
		return "List(elts=[" + astDumpList(e.kids) + "], ctx=Load())"
	case "not":
		return "UnaryOp(op=Not(), operand=" + astDumpExpr(kid(e, 0)) + ")"
	case "unary":
		op := "UAdd"
		switch e.lit {
		case "-":
			op = "USub"
		case "~":
			op = "Invert"
		}
		return "UnaryOp(op=" + op + "(), operand=" + astDumpExpr(kid(e, 0)) + ")"
	case "bin":
		return "BinOp(left=" + astDumpExpr(kid(e, 0)) + ", op=" + binOpName(e.lit) + "(), right=" + astDumpExpr(kid(e, 1)) + ")"
	case "and":
		return "BoolOp(op=And(), values=[" + astDumpList(e.kids) + "])"
	case "or":
		return "BoolOp(op=Or(), values=[" + astDumpList(e.kids) + "])"
	case "compare":
		ops := strings.Split(e.lit, ",")
		var opDump []string
		for _, op := range ops {
			opDump = append(opDump, cmpOpName(op)+"()")
		}
		left := "None"
		var comps []*px
		if len(e.kids) > 0 {
			left = astDumpExpr(e.kids[0])
			comps = e.kids[1:]
		}
		return "Compare(left=" + left + ", ops=[" + strings.Join(opDump, ", ") + "], comparators=[" + astDumpList(comps) + "])"
	case "ifexp":
		return "IfExp(test=" + astDumpExpr(kid(e, 0)) + ", body=" + astDumpExpr(kid(e, 1)) + ", orelse=" + astDumpExpr(kid(e, 2)) + ")"
	default:
		return "Constant(value=" + pyQuote(unparse(e)) + ")"
	}
}

func kid(e *px, i int) *px {
	if e == nil || i >= len(e.kids) {
		return nil
	}
	return e.kids[i]
}

func astDumpList(kids []*px) string {
	parts := make([]string, len(kids))
	for i, k := range kids {
		parts[i] = astDumpExpr(k)
	}
	return strings.Join(parts, ", ")
}

func pyQuote(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		return "\"" + s + "\""
	}
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "'", "\\'") + "'"
}

func binOpName(sym string) string {
	switch sym {
	case "+":
		return "Add"
	case "-":
		return "Sub"
	case "*":
		return "Mult"
	case "/":
		return "Div"
	case "//":
		return "FloorDiv"
	case "%":
		return "Mod"
	case "**":
		return "Pow"
	case "<<":
		return "LShift"
	case ">>":
		return "RShift"
	case "|":
		return "BitOr"
	case "^":
		return "BitXor"
	case "&":
		return "BitAnd"
	case "@":
		return "MatMult"
	default:
		return "Add"
	}
}

func cmpOpName(sym string) string {
	switch sym {
	case "==":
		return "Eq"
	case "!=":
		return "NotEq"
	case "<":
		return "Lt"
	case "<=":
		return "LtE"
	case ">":
		return "Gt"
	case ">=":
		return "GtE"
	case "is":
		return "Is"
	case "is not":
		return "IsNot"
	case "in":
		return "In"
	case "not in":
		return "NotIn"
	default:
		return "Eq"
	}
}

func isDocstring(s pyStmt) bool {
	return s.kind == skExpr && s.value != nil && s.value.op == "str"
}

func dumpStmt(b *strings.Builder, s pyStmt) {
	b.WriteString(fmt.Sprintf("k%d ", s.kind))
	if s.async {
		b.WriteString("async ")
	}
	if s.name != "" {
		b.WriteString(s.name)
		b.WriteByte(' ')
	}
	if s.test != nil {
		b.WriteString(unparse(s.test))
		b.WriteByte(' ')
	}
	if s.ann != nil {
		b.WriteString(unparse(s.ann))
		b.WriteByte(' ')
	}
	if s.value != nil {
		b.WriteString(unparse(s.value))
		b.WriteByte(' ')
	}
	for _, e := range s.withExprs {
		b.WriteString(unparse(e))
		b.WriteByte(' ')
	}
	if len(s.handlers) > 0 || len(s.body) > 0 || len(s.orelse) > 0 || len(s.final) > 0 || len(s.cases) > 0 {
		b.WriteByte('{')
		for _, h := range s.handlers {
			b.WriteString("except ")
			b.WriteString(strings.Join(h.names, "|"))
			b.WriteByte(' ')
			for _, st := range h.body {
				dumpStmt(b, st)
			}
		}
		for _, st := range s.body {
			dumpStmt(b, st)
		}
		b.WriteString(" else ")
		for _, st := range s.orelse {
			dumpStmt(b, st)
		}
		b.WriteString(" finally ")
		for _, st := range s.final {
			dumpStmt(b, st)
		}
		for _, cs := range s.cases {
			b.WriteString(" case ")
			b.WriteString(unparse(cs.guard))
			for _, st := range cs.body {
				dumpStmt(b, st)
			}
		}
		b.WriteByte('}')
	}
}

func comparePyFunc(logical, name string, before, after pyFacts) []Candidate {
	var out []Candidate
	if before.ifCount != after.ifCount || before.ifElse != after.ifElse || before.tryElse != after.tryElse || before.loopElse != after.loopElse || before.retCount != after.retCount {
		out = append(out, makeCandidate(logical, name, "python.control_flow_return_change", fmtFlow(before), fmtFlow(after)))
	}
	if !sameCounts(before.cond, after.cond) {
		out = append(out, makeCandidate(logical, name, "python.condition_operator_change", fmtCounter(before.cond), fmtCounter(after.cond)))
	}
	if !sameStringSlice(before.contexts, after.contexts) {
		out = append(out, makeCandidate(logical, name, "python.context_manager_change", joinOrNone(before.contexts, ", "), joinOrNone(after.contexts, ", ")))
	}
	if !sameStringSlice(before.finallyHash, after.finallyHash) {
		out = append(out, makeCandidate(logical, name, "python.finally_body_change", joinOrNone(before.finallyHash, ", "), joinOrNone(after.finallyHash, ", ")))
	}
	if !sameExcept(before.exceptTypes, after.exceptTypes) {
		out = append(out, makeCandidate(logical, name, "python.except_types_change", fmtExcept(before.exceptTypes), fmtExcept(after.exceptTypes)))
	}
	if before.exceptRaise != after.exceptRaise {
		out = append(out, makeCandidate(
			logical, name, "python.except_raise_change",
			fmt.Sprintf("except_raise=%d", before.exceptRaise),
			fmt.Sprintf("except_raise=%d", after.exceptRaise),
		))
	}
	return out
}

func fmtFlow(f pyFacts) string {
	return fmt.Sprintf("if=%d, if_else=%d, try_else=%d, loop_else=%d, return=%d", f.ifCount, f.ifElse, f.tryElse, f.loopElse, f.retCount)
}

func fmtExcept(groups [][][]string) string {
	if len(groups) == 0 {
		return "none"
	}
	parts := make([]string, len(groups))
	for i, handlers := range groups {
		hs := make([]string, len(handlers))
		for j, h := range handlers {
			hs[j] = strings.Join(h, "|")
		}
		parts[i] = "try(" + strings.Join(hs, ", ") + ")"
	}
	return strings.Join(parts, "; ")
}

func sameExcept(a, b [][][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if !sameStringSlice(a[i][j], b[i][j]) {
				return false
			}
		}
	}
	return true
}

func uniqSorted(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sortStrings(out)
	return out
}
