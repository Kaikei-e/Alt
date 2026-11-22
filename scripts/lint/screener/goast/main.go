package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
)

type CallArg struct {
	Func  string `json:"func"`
	Value string `json:"value"`
}

type FuncSummary struct {
	Name              string              `json:"name"`
	CondOps           map[string]int      `json:"cond_ops"`
	ReturnCount       int                 `json:"return_count"`
	DeferCount        int                 `json:"defer_count"`
	HTTPServerConfigs []map[string]string `json:"http_server_configs"`
	FileModes         []CallArg           `json:"file_modes"`
	BodyLimits        []string            `json:"body_limits"`
	ReverseProxyModes []string            `json:"reverse_proxy_modes"`
	OutputEscapers    map[string]int      `json:"output_escapers"`
	SlogDiscardCount  int                 `json:"slog_discard_count"`
	SlogOtherHandlers int                 `json:"slog_other_handlers"`
}

type FileSummary struct {
	Path      string                  `json:"path"`
	ParseErr  string                  `json:"parse_err,omitempty"`
	Functions map[string]*FuncSummary `json:"functions"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: goast <file.go> [file2.go ...]")
		os.Exit(2)
	}

	summaries := make([]FileSummary, 0, len(os.Args)-1)
	for _, path := range os.Args[1:] {
		summaries = append(summaries, analyzeFile(path))
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(summaries); err != nil {
		fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		os.Exit(2)
	}
}

func analyzeFile(path string) FileSummary {
	fset := token.NewFileSet()
	src, err := os.ReadFile(path)
	if err != nil {
		return FileSummary{
			Path:      path,
			ParseErr:  fmt.Sprintf("read error: %v", err),
			Functions: map[string]*FuncSummary{},
		}
	}

	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return FileSummary{
			Path:      path,
			ParseErr:  fmt.Sprintf("parse error: %v", err),
			Functions: map[string]*FuncSummary{},
		}
	}

	intConsts := collectIntConsts(file)
	functions := map[string]*FuncSummary{}
	nameCounts := map[string]int{}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		baseName := funcKey(fset, fn)
		nameCounts[baseName]++
		key := baseName
		if nameCounts[baseName] > 1 {
			key = fmt.Sprintf("%s#%d", baseName, nameCounts[baseName])
		}
		functions[key] = analyzeFunc(fset, fn, key, intConsts)
	}

	return FileSummary{
		Path:      path,
		Functions: functions,
	}
}

func funcKey(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recvType := compactFormat(fset, fn.Recv.List[0].Type)
	return fmt.Sprintf("(%s).%s", recvType, fn.Name.Name)
}

func compactFormat(fset *token.FileSet, node any) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

func collectIntConsts(file *ast.File) map[string]int64 {
	rawExprs := map[string]ast.Expr{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range vs.Names {
				if i < len(vs.Values) {
					rawExprs[ident.Name] = vs.Values[i]
				}
			}
		}
	}

	resolved := map[string]int64{}
	for pass := 0; pass < len(rawExprs)+1; pass++ {
		progressed := false
		for name, expr := range rawExprs {
			if _, done := resolved[name]; done {
				continue
			}
			if val, ok := evalIntConst(expr, resolved); ok {
				resolved[name] = val
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return resolved
}

func evalIntConst(expr ast.Expr, consts map[string]int64) (int64, bool) {
	val, ok := evalConstant(expr, consts)
	if !ok || val.Kind() != constant.Int {
		return 0, false
	}
	i64, exact := constant.Int64Val(val)
	return i64, exact
}

func evalConstant(expr ast.Expr, consts map[string]int64) (constant.Value, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return evalConstant(e.X, consts)
	case *ast.BasicLit:
		if e.Kind == token.INT {
			v := constant.MakeFromLiteral(e.Value, token.INT, 0)
			if v.Kind() == constant.Int {
				return v, true
			}
		}
		return nil, false
	case *ast.Ident:
		if v, ok := consts[e.Name]; ok {
			return constant.MakeInt64(v), true
		}
		return nil, false
	case *ast.UnaryExpr:
		x, ok := evalConstant(e.X, consts)
		if !ok {
			return nil, false
		}
		switch e.Op {
		case token.ADD, token.SUB, token.XOR:
			return constant.UnaryOp(e.Op, x, 0), true
		default:
			return nil, false
		}
	case *ast.BinaryExpr:
		x, okX := evalConstant(e.X, consts)
		y, okY := evalConstant(e.Y, consts)
		if !okX || !okY {
			return nil, false
		}
		switch e.Op {
		case token.SHL, token.SHR:
			s, exact := constant.Uint64Val(y)
			if !exact || s > 62 {
				return nil, false
			}
			return constant.Shift(x, e.Op, uint(s)), true
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.AND, token.OR, token.XOR:
			if (e.Op == token.QUO || e.Op == token.REM) && constant.Sign(y) == 0 {
				return nil, false
			}
			return constant.BinaryOp(x, e.Op, y), true
		default:
			return nil, false
		}
	default:
		return nil, false
	}
}

func analyzeFunc(
	fset *token.FileSet,
	fn *ast.FuncDecl,
	key string,
	intConsts map[string]int64,
) *FuncSummary {
	summary := &FuncSummary{
		Name:              key,
		CondOps:           map[string]int{},
		HTTPServerConfigs: []map[string]string{},
		FileModes:         []CallArg{},
		BodyLimits:        []string{},
		ReverseProxyModes: []string{},
		OutputEscapers:    map[string]int{},
	}

	rpModes := map[string]struct{}{}

	// Direct return/defer count (excluding nested FuncLit closures).
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		switch n.(type) {
		case *ast.ReturnStmt:
			summary.ReturnCount++
		case *ast.DeferStmt:
			summary.DeferCount++
		}
		return true
	})

	// Full body inspection for conditions, server configs, file modes, limits, proxy, escaping, slog.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch node := n.(type) {
		case *ast.IfStmt:
			collectCondOps(node.Cond, summary.CondOps)
		case *ast.ForStmt:
			if node.Cond != nil {
				collectCondOps(node.Cond, summary.CondOps)
			}
		case *ast.SwitchStmt:
			if node.Tag != nil {
				collectCondOps(node.Tag, summary.CondOps)
			}
		case *ast.CaseClause:
			for _, expr := range node.List {
				collectCondOps(expr, summary.CondOps)
			}
		case *ast.CompositeLit:
			inspectCompositeLit(fset, node, summary, rpModes)
		case *ast.AssignStmt:
			inspectAssignStmt(fset, node, summary, rpModes)
		case *ast.CallExpr:
			inspectCallExpr(fset, node, intConsts, summary, rpModes)
		case *ast.SelectorExpr:
			inspectSelectorExpr(node, summary)
		}
		return true
	})

	if len(rpModes) > 0 {
		modes := make([]string, 0, len(rpModes))
		for m := range rpModes {
			modes = append(modes, m)
		}
		sort.Strings(modes)
		summary.ReverseProxyModes = modes
	}

	return summary
}

func unwrapParens(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

func isErrorsIsCall(expr ast.Expr) bool {
	call, ok := unwrapParens(expr).(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "errors" && sel.Sel.Name == "Is"
}

func collectCondOps(expr ast.Expr, ops map[string]int) {
	if expr == nil {
		return
	}
	expr = unwrapParens(expr)
	if isErrorsIsCall(expr) {
		ops["=="]++
		return
	}

	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			if isErrorsIsCall(e.X) {
				ops["!="]++
				return
			}
			ops["!"]++
		}
		collectCondOps(e.X, ops)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ, token.LAND, token.LOR:
			ops[e.Op.String()]++
		}
		collectCondOps(e.X, ops)
		collectCondOps(e.Y, ops)
	}
}

func selectorPkgAndName(expr ast.Expr) (string, string) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name, sel.Sel.Name
	}
	return "", sel.Sel.Name
}

func inspectCompositeLit(
	fset *token.FileSet,
	cl *ast.CompositeLit,
	summary *FuncSummary,
	rpModes map[string]struct{},
) {
	pkg, name := selectorPkgAndName(cl.Type)
	if (pkg == "http" && name == "Server") || (pkg == "httpserver" && name == "Config") {
		fields := map[string]string{}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyStr := compactFormat(fset, kv.Key)
			valStr := compactFormat(fset, kv.Value)
			fields[keyStr] = valStr
		}
		summary.HTTPServerConfigs = append(summary.HTTPServerConfigs, fields)
	}
	if name == "ReverseProxy" {
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyStr := compactFormat(fset, kv.Key)
			if keyStr == "Director" || keyStr == "Rewrite" {
				rpModes[keyStr] = struct{}{}
			}
		}
	}
}

var httpServerTimeoutFields = map[string]bool{
	"ReadTimeout":       true,
	"ReadHeaderTimeout": true,
	"WriteTimeout":      true,
	"IdleTimeout":       true,
	"MaxHeaderBytes":    true,
}

func inspectAssignStmt(
	fset *token.FileSet,
	assign *ast.AssignStmt,
	summary *FuncSummary,
	rpModes map[string]struct{},
) {
	for i, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		fieldName := sel.Sel.Name
		if fieldName == "Director" || fieldName == "Rewrite" {
			rpModes[fieldName] = struct{}{}
		}
		if httpServerTimeoutFields[fieldName] && i < len(assign.Rhs) {
			summary.HTTPServerConfigs = append(summary.HTTPServerConfigs, map[string]string{
				"assign." + fieldName: compactFormat(fset, assign.Rhs[i]),
			})
		}
	}
}

var outputEscapeFuncs = map[string]bool{
	"html.EscapeString":         true,
	"url.QueryEscape":           true,
	"url.PathEscape":            true,
	"template.HTMLEscapeString": true,
	"template.JSEscapeString":   true,
}

func inspectCallExpr(
	fset *token.FileSet,
	call *ast.CallExpr,
	intConsts map[string]int64,
	summary *FuncSummary,
	rpModes map[string]struct{},
) {
	pkg, name := selectorPkgAndName(call.Fun)
	if name == "" {
		if id, ok := call.Fun.(*ast.Ident); ok {
			name = id.Name
		}
	}

	// File mode arguments.
	modeArgIdx := -1
	switch name {
	case "WriteFile", "OpenFile":
		modeArgIdx = 2
	case "MkdirAll", "Mkdir", "Chmod":
		modeArgIdx = 1
	}
	if modeArgIdx >= 0 && len(call.Args) > modeArgIdx {
		arg := call.Args[modeArgIdx]
		valStr := compactFormat(fset, arg)
		if v, ok := evalIntConst(arg, intConsts); ok {
			valStr = fmt.Sprintf("0o%o", v)
		}
		fnLabel := name
		if pkg != "" {
			fnLabel = pkg + "." + name
		}
		summary.FileModes = append(summary.FileModes, CallArg{Func: fnLabel, Value: valStr})
	}

	// Echo/middleware BodyLimit(...)
	if name == "BodyLimit" && len(call.Args) >= 1 {
		arg := call.Args[0]
		valStr := compactFormat(fset, arg)
		if v, ok := evalIntConst(arg, intConsts); ok {
			valStr = strconv.FormatInt(v, 10)
		}
		summary.BodyLimits = append(summary.BodyLimits, valStr)
	}

	// ReverseProxy markers.
	switch name {
	case "NewSingleHostReverseProxy", "SetXForwarded", "SetURL":
		rpModes[name] = struct{}{}
	}

	// Output escaping wrappers.
	if pkg != "" && name != "" {
		full := pkg + "." + name
		if outputEscapeFuncs[full] {
			summary.OutputEscapers[full]++
		}
	}
}

func inspectSelectorExpr(sel *ast.SelectorExpr, summary *FuncSummary) {
	pkg, name := selectorPkgAndName(sel)
	if pkg != "slog" {
		return
	}
	switch name {
	case "DiscardHandler":
		summary.SlogDiscardCount++
	case "NewTextHandler", "NewJSONHandler":
		summary.SlogOtherHandlers++
	}
}
