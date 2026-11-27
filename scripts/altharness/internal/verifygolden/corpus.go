package verifygolden

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CorpusResult is the outcome of evaluating effective runtime configuration
// drift between a corpus before.go and after.go file.
type CorpusResult struct {
	CaseID     string   `json:"case_id,omitempty"`
	Equal      bool     `json:"equal"`
	Signal     string   `json:"signal,omitempty"`
	Dimensions []string `json:"dimensions,omitempty"`
	BeforeJSON string   `json:"before_json"`
	AfterJSON  string   `json:"after_json"`
	Diff       string   `json:"diff,omitempty"`
}

// EffectiveSnippetConfig captures the normalized effective runtime
// configuration extracted from a standalone Go source file.
type EffectiveSnippetConfig struct {
	BodyLimits      []BodyLimitEntry      `json:"body_limits"`
	CORSOrigins     []string              `json:"cors_origins"`
	FileModes       []FileModeEntry       `json:"file_modes"`
	HTTPServers     []HTTPServerEntry     `json:"http_servers"`
	OutputEscapers  []string              `json:"output_escapers"`
	ResponseHeaders map[string]string     `json:"response_headers"`
	ReverseProxies  []ReverseProxyEntry   `json:"reverse_proxies"`
	Routes          []RouteMiddlewareSpec `json:"routes"`
}

// HTTPServerEntry captures normalized timeouts of a constructed http.Server.
type HTTPServerEntry struct {
	Function          string `json:"function"`
	IdleTimeout       string `json:"idle_timeout"`
	MaxHeaderBytes    int64  `json:"max_header_bytes"`
	ReadHeaderTimeout string `json:"read_header_timeout"`
	ReadTimeout       string `json:"read_timeout"`
	WriteTimeout      string `json:"write_timeout"`
}

// FileModeEntry captures an effective directory/file permission call.
type FileModeEntry struct {
	Function string `json:"function"`
	Call     string `json:"call"`
	Mode     string `json:"mode"`
}

// BodyLimitEntry captures an effective Echo BodyLimit byte value.
type BodyLimitEntry struct {
	Function   string `json:"function"`
	LimitBytes int64  `json:"limit_bytes"`
	RawExpr    string `json:"raw_expr,omitempty"`
}

// ReverseProxyEntry captures effective upstream forwarding semantics.
type ReverseProxyEntry struct {
	Function                   string   `json:"function"`
	PreservesInboundXFF        bool     `json:"preserves_inbound_x_forwarded_for"`
	StrippedHeaders            []string `json:"stripped_headers"`
	SynthesizesXForwardedHost  bool     `json:"synthesizes_x_forwarded_host"`
	SynthesizesXForwardedProto bool     `json:"synthesizes_x_forwarded_proto"`
	UsesDirector               bool     `json:"uses_director"`
	UsesRewrite                bool     `json:"uses_rewrite"`
	UsesSetXForwarded          bool     `json:"uses_set_x_forwarded"`
	FlushInterval              string   `json:"flush_interval,omitempty"`
}

// RouteMiddlewareSpec captures a route registration extracted from a Go snippet.
type RouteMiddlewareSpec struct {
	Function string `json:"function"`
	Method   string `json:"method"`
	Path     string `json:"path"`
}

// EvaluateCorpusCase extracts effective configuration snapshots from beforePath
// and afterPath, canonicalizes both to sorted JSON, and reports any drift.
func EvaluateCorpusCase(ctx context.Context, caseID, beforePath, afterPath string) (CorpusResult, error) {
	if ctx == nil {
		return CorpusResult{}, errors.New("verifygolden: nil context")
	}
	if err := ctx.Err(); err != nil {
		return CorpusResult{}, err
	}
	beforeCfg, err := ExtractEffectiveConfigFromGo(beforePath)
	if err != nil {
		return CorpusResult{}, fmt.Errorf("extract before %q: %w", beforePath, err)
	}
	afterCfg, err := ExtractEffectiveConfigFromGo(afterPath)
	if err != nil {
		return CorpusResult{}, fmt.Errorf("extract after %q: %w", afterPath, err)
	}

	beforeBytes, err := CanonicalizeAny(beforeCfg)
	if err != nil {
		return CorpusResult{}, fmt.Errorf("canonicalize before: %w", err)
	}
	afterBytes, err := CanonicalizeAny(afterCfg)
	if err != nil {
		return CorpusResult{}, fmt.Errorf("canonicalize after: %w", err)
	}

	res := CorpusResult{
		CaseID:     caseID,
		BeforeJSON: string(beforeBytes),
		AfterJSON:  string(afterBytes),
	}
	if bytes.Equal(beforeBytes, afterBytes) {
		res.Equal = true
		return res, nil
	}

	res.Equal = false
	res.Signal = CorpusDiffSignal
	res.Dimensions = changedDimensions(beforeCfg, afterCfg)
	res.Diff = formatLineDiff(res.BeforeJSON, res.AfterJSON)
	return res, nil
}

// ExtractEffectiveConfigFromGo parses a Go source file and extracts its
// normalized effective runtime configuration across VERIFY-03 dimensions.
func ExtractEffectiveConfigFromGo(path string) (EffectiveSnippetConfig, error) {
	src, err := readRegularFileRooted(path)
	if err != nil {
		return EffectiveSnippetConfig{}, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return EffectiveSnippetConfig{}, err
	}

	intConsts := collectIntConsts(file)
	cfg := EffectiveSnippetConfig{
		BodyLimits:      []BodyLimitEntry{},
		CORSOrigins:     []string{},
		FileModes:       []FileModeEntry{},
		HTTPServers:     []HTTPServerEntry{},
		OutputEscapers:  []string{},
		ResponseHeaders: map[string]string{},
		ReverseProxies:  []ReverseProxyEntry{},
		Routes:          []RouteMiddlewareSpec{},
	}

	escapers := map[string]struct{}{}
	corsSet := map[string]struct{}{}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		fnName := fn.Name.Name
		inspectFunction(fset, fnName, fn.Body, intConsts, &cfg, escapers, corsSet)
	}

	for esc := range escapers {
		cfg.OutputEscapers = append(cfg.OutputEscapers, esc)
	}
	sort.Strings(cfg.OutputEscapers)

	for origin := range corsSet {
		cfg.CORSOrigins = append(cfg.CORSOrigins, origin)
	}
	sort.Strings(cfg.CORSOrigins)

	sort.Slice(cfg.HTTPServers, func(i, j int) bool {
		if cfg.HTTPServers[i].Function != cfg.HTTPServers[j].Function {
			return cfg.HTTPServers[i].Function < cfg.HTTPServers[j].Function
		}
		return cfg.HTTPServers[i].IdleTimeout < cfg.HTTPServers[j].IdleTimeout
	})
	sort.Slice(cfg.FileModes, func(i, j int) bool {
		if cfg.FileModes[i].Function != cfg.FileModes[j].Function {
			return cfg.FileModes[i].Function < cfg.FileModes[j].Function
		}
		if cfg.FileModes[i].Call != cfg.FileModes[j].Call {
			return cfg.FileModes[i].Call < cfg.FileModes[j].Call
		}
		return cfg.FileModes[i].Mode < cfg.FileModes[j].Mode
	})
	sort.Slice(cfg.BodyLimits, func(i, j int) bool {
		if cfg.BodyLimits[i].Function != cfg.BodyLimits[j].Function {
			return cfg.BodyLimits[i].Function < cfg.BodyLimits[j].Function
		}
		return cfg.BodyLimits[i].LimitBytes < cfg.BodyLimits[j].LimitBytes
	})
	sort.Slice(cfg.ReverseProxies, func(i, j int) bool {
		return cfg.ReverseProxies[i].Function < cfg.ReverseProxies[j].Function
	})
	sort.Slice(cfg.Routes, func(i, j int) bool {
		if cfg.Routes[i].Path != cfg.Routes[j].Path {
			return cfg.Routes[i].Path < cfg.Routes[j].Path
		}
		return cfg.Routes[i].Method < cfg.Routes[j].Method
	})

	return cfg, nil
}

func inspectFunction(
	fset *token.FileSet,
	fnName string,
	body *ast.BlockStmt,
	intConsts map[string]int64,
	cfg *EffectiveSnippetConfig,
	escapers map[string]struct{},
	corsSet map[string]struct{},
) {
	var (
		hasProxy            bool
		usesDirector        bool
		usesRewrite         bool
		usesSetXForwarded   bool
		preservesInboundXFF bool
		flushInterval       string
		strippedHeaders     = map[string]struct{}{}
	)

	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch node := n.(type) {
		case *ast.CompositeLit:
			pkg, name := selectorPkgName(node.Type)
			if (pkg == "http" && name == "Server") || (pkg == "httpserver" && name == "Config") {
				cfg.HTTPServers = append(cfg.HTTPServers, extractHTTPServer(fset, fnName, node, intConsts))
			}
			if name == "ReverseProxy" {
				hasProxy = true
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key := exprString(fset, kv.Key)
					switch key {
					case "Director":
						usesDirector = true
					case "Rewrite":
						usesRewrite = true
					case "FlushInterval":
						flushInterval = exprString(fset, kv.Value)
					}
				}
			}
			if name == "CORSConfig" {
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok || exprString(fset, kv.Key) != "AllowOrigins" {
						continue
					}
					if sliceLit, ok := kv.Value.(*ast.CompositeLit); ok {
						for _, elem := range sliceLit.Elts {
							if s, ok := stringLitVal(elem); ok {
								corsSet[s] = struct{}{}
							}
						}
					}
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				switch sel.Sel.Name {
				case "Director":
					hasProxy = true
					usesDirector = true
				case "Rewrite":
					hasProxy = true
					usesRewrite = true
				case "FlushInterval":
					if i < len(node.Rhs) {
						flushInterval = exprString(fset, node.Rhs[i])
					}
				}
			}
		case *ast.IndexExpr:
			// Check for explicit preservation of inbound X-Forwarded-For in Rewrite.
			if sel, ok := node.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Header" {
				if inSel, ok := sel.X.(*ast.SelectorExpr); ok && inSel.Sel.Name == "In" {
					if s, ok := stringLitVal(node.Index); ok && strings.EqualFold(s, "X-Forwarded-For") {
						preservesInboundXFF = true
					}
				}
			}
		case *ast.CallExpr:
			pkg, name := selectorPkgName(node.Fun)
			if name == "" {
				if id, ok := node.Fun.(*ast.Ident); ok {
					name = id.Name
				}
			}

			// File mode extraction (os.MkdirAll, os.WriteFile, etc.).
			modeIdx := -1
			switch name {
			case "WriteFile", "OpenFile":
				modeIdx = 2
			case "MkdirAll", "Mkdir", "Chmod":
				modeIdx = 1
			}
			if modeIdx >= 0 && len(node.Args) > modeIdx {
				callName := name
				if pkg != "" {
					callName = pkg + "." + name
				}
				modeStr := exprString(fset, node.Args[modeIdx])
				if val, ok := evalIntConst(node.Args[modeIdx], intConsts); ok {
					modeStr = fmt.Sprintf("%04o", val)
				}
				cfg.FileModes = append(cfg.FileModes, FileModeEntry{
					Function: fnName,
					Call:     callName,
					Mode:     modeStr,
				})
			}

			// Echo BodyLimit extraction.
			if name == "BodyLimit" && len(node.Args) >= 1 {
				entry := BodyLimitEntry{Function: fnName}
				if val, ok := evalIntConst(node.Args[0], intConsts); ok {
					entry.LimitBytes = val
				} else {
					entry.RawExpr = exprString(fset, node.Args[0])
				}
				cfg.BodyLimits = append(cfg.BodyLimits, entry)
			}

			// ReverseProxy helpers & header stripping.
			switch name {
			case "NewSingleHostReverseProxy":
				hasProxy = true
				usesDirector = true
			case "SetXForwarded":
				hasProxy = true
				usesSetXForwarded = true
			case "Del":
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					if hdrSel, ok := sel.X.(*ast.SelectorExpr); ok && hdrSel.Sel.Name == "Header" && len(node.Args) == 1 {
						if s, ok := stringLitVal(node.Args[0]); ok {
							strippedHeaders[s] = struct{}{}
						}
					}
				}
			case "Set":
				// Response headers like w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					if hdrCall, ok := sel.X.(*ast.CallExpr); ok {
						_, hdrFn := selectorPkgName(hdrCall.Fun)
						if hdrFn == "Header" && len(node.Args) == 2 {
							if k, okK := stringLitVal(node.Args[0]); okK {
								if v, okV := stringLitVal(node.Args[1]); okV {
									cfg.ResponseHeaders[k] = v
								}
							}
						}
					}
				}
			case "GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD":
				if len(node.Args) >= 2 {
					if routePath, ok := stringLitVal(node.Args[0]); ok {
						cfg.Routes = append(cfg.Routes, RouteMiddlewareSpec{
							Function: fnName,
							Method:   name,
							Path:     routePath,
						})
					}
				}
			}

			// Output escapers (e.g. html.EscapeString).
			if pkg != "" && name != "" {
				full := pkg + "." + name
				switch full {
				case "html.EscapeString", "url.QueryEscape", "url.PathEscape", "template.HTMLEscapeString", "template.JSEscapeString":
					escapers[full] = struct{}{}
				}
			}
		}
		return true
	})

	if hasProxy {
		// In Go stdlib net/http/httputil:
		// - Director preserves inbound X-Forwarded-For (appending client IP) and never
		//   synthesizes X-Forwarded-Host / X-Forwarded-Proto.
		// - Rewrite strips inbound X-Forwarded-* before running Rewrite; calling
		//   pr.SetXForwarded() without copying pr.In.Header["X-Forwarded-For"] drops
		//   inbound XFF AND synthesizes X-Forwarded-Host and X-Forwarded-Proto.
		preservesXFF := false
		if usesDirector && !usesRewrite {
			preservesXFF = true
		} else if usesRewrite {
			preservesXFF = preservesInboundXFF
		}
		// Detect slice-based header preservation loop like:
		//   for _, h := range []string{"Forwarded", "X-Forwarded-For", ...} { pr.Out.Header[h] = slices.Clone(pr.In.Header[h]) }
		if usesRewrite && !preservesXFF && bodyPreservesXFFInLoop(body) {
			preservesXFF = true
		}
		stripped := make([]string, 0, len(strippedHeaders))
		for h := range strippedHeaders {
			stripped = append(stripped, h)
		}
		sort.Strings(stripped)
		cfg.ReverseProxies = append(cfg.ReverseProxies, ReverseProxyEntry{
			Function:                   fnName,
			PreservesInboundXFF:        preservesXFF,
			StrippedHeaders:            stripped,
			SynthesizesXForwardedHost:  usesSetXForwarded,
			SynthesizesXForwardedProto: usesSetXForwarded,
			UsesDirector:               usesDirector,
			UsesRewrite:                usesRewrite,
			UsesSetXForwarded:          usesSetXForwarded,
			FlushInterval:              flushInterval,
		})
	}
}

func bodyPreservesXFFInLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found || n == nil {
			return false
		}
		rs, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		cl, ok := rs.X.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range cl.Elts {
			if s, ok := stringLitVal(elt); ok && strings.EqualFold(s, "X-Forwarded-For") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func extractHTTPServer(fset *token.FileSet, fnName string, cl *ast.CompositeLit, intConsts map[string]int64) HTTPServerEntry {
	entry := HTTPServerEntry{
		Function:          fnName,
		ReadHeaderTimeout: "0s",
		ReadTimeout:       "0s",
		WriteTimeout:      "0s",
		IdleTimeout:       "0s",
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := exprString(fset, kv.Key)
		switch key {
		case "ReadHeaderTimeout":
			entry.ReadHeaderTimeout = evalDurationString(fset, kv.Value, intConsts)
		case "ReadTimeout":
			entry.ReadTimeout = evalDurationString(fset, kv.Value, intConsts)
		case "WriteTimeout":
			entry.WriteTimeout = evalDurationString(fset, kv.Value, intConsts)
		case "IdleTimeout":
			entry.IdleTimeout = evalDurationString(fset, kv.Value, intConsts)
		case "MaxHeaderBytes":
			if val, ok := evalIntConst(kv.Value, intConsts); ok {
				entry.MaxHeaderBytes = val
			}
		}
	}
	return entry
}

func evalDurationString(fset *token.FileSet, expr ast.Expr, intConsts map[string]int64) string {
	if ns, ok := evalDurationNanos(expr, intConsts); ok {
		return time.Duration(ns).String()
	}
	return exprString(fset, expr)
}

func evalDurationNanos(expr ast.Expr, intConsts map[string]int64) (int64, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return evalDurationNanos(e.X, intConsts)
	case *ast.SelectorExpr:
		pkg, name := selectorPkgName(e)
		if pkg == "time" {
			switch name {
			case "Nanosecond":
				return int64(time.Nanosecond), true
			case "Microsecond":
				return int64(time.Microsecond), true
			case "Millisecond":
				return int64(time.Millisecond), true
			case "Second":
				return int64(time.Second), true
			case "Minute":
				return int64(time.Minute), true
			case "Hour":
				return int64(time.Hour), true
			}
		}
		return 0, false
	case *ast.BinaryExpr:
		lx, okL := evalDurationNanos(e.X, intConsts)
		rx, okR := evalDurationNanos(e.Y, intConsts)
		if okL && okR {
			switch e.Op {
			case token.MUL:
				return lx * rx, true
			case token.ADD:
				return lx + rx, true
			case token.SUB:
				return lx - rx, true
			case token.QUO:
				if rx != 0 {
					return lx / rx, true
				}
			default:
				return 0, false
			}
		}
		return 0, false
	default:
		return evalIntConst(expr, intConsts)
	}
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
	for range len(rawExprs) + 1 {
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
	val, ok := evalConstValue(expr, consts)
	if !ok || val.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(val)
}

func evalConstValue(expr ast.Expr, consts map[string]int64) (constant.Value, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return evalConstValue(e.X, consts)
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
		x, ok := evalConstValue(e.X, consts)
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
		x, okX := evalConstValue(e.X, consts)
		y, okY := evalConstValue(e.Y, consts)
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

func selectorPkgName(expr ast.Expr) (string, string) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name, sel.Sel.Name
	}
	return "", sel.Sel.Name
}

func stringLitVal(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func exprString(fset *token.FileSet, node any) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

func changedDimensions(before, after EffectiveSnippetConfig) []string {
	type pair struct {
		name string
		a, b any
	}
	checks := []pair{
		{"body_limits", before.BodyLimits, after.BodyLimits},
		{"cors_origins", before.CORSOrigins, after.CORSOrigins},
		{"file_modes", before.FileModes, after.FileModes},
		{"http_servers", before.HTTPServers, after.HTTPServers},
		{"output_escapers", before.OutputEscapers, after.OutputEscapers},
		{"response_headers", before.ResponseHeaders, after.ResponseHeaders},
		{"reverse_proxies", before.ReverseProxies, after.ReverseProxies},
		{"routes", before.Routes, after.Routes},
	}
	var dims []string
	for _, c := range checks {
		ab, _ := CanonicalizeAny(c.a)
		bb, _ := CanonicalizeAny(c.b)
		if !bytes.Equal(ab, bb) {
			dims = append(dims, c.name)
		}
	}
	return dims
}

func formatLineDiff(before, after string) string {
	bLines := strings.Split(strings.TrimRight(before, "\n"), "\n")
	aLines := strings.Split(strings.TrimRight(after, "\n"), "\n")
	maxLen := len(bLines)
	if len(aLines) > maxLen {
		maxLen = len(aLines)
	}
	var out strings.Builder
	for i := range maxLen {
		var bl, al string
		hasB, hasA := i < len(bLines), i < len(aLines)
		if hasB {
			bl = bLines[i]
		}
		if hasA {
			al = aLines[i]
		}
		if hasB && hasA && bl == al {
			continue
		}
		if hasB {
			fmt.Fprintf(&out, "- %s\n", bl)
		}
		if hasA {
			fmt.Fprintf(&out, "+ %s\n", al)
		}
	}
	return strings.TrimRight(out.String(), "\n")
}
