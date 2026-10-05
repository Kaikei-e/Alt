package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

type callSite struct {
	pos  token.Pos
	args []ast.Expr
}

// callsInMain indexes `pkg.Fn(...)` and bare `fn(...)` calls inside func main,
// including deferred closures, by their source position.
func callsInMain(t *testing.T) map[string][]callSite {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("main.go has no func main")
	}
	calls := map[string][]callSite{}
	ast.Inspect(mainFn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := fun.X.(*ast.Ident); ok {
				name = pkg.Name + "." + fun.Sel.Name
			}
		case *ast.Ident:
			name = fun.Name
		}
		if name != "" {
			calls[name] = append(calls[name], callSite{pos: call.Pos(), args: call.Args})
		}
		return true
	})
	return calls
}

func stringArg(args []ast.Expr, i int) string {
	if i >= len(args) {
		return ""
	}
	lit, ok := args[i].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

func onlyCall(t *testing.T, calls map[string][]callSite, name string) callSite {
	t.Helper()
	sites := calls[name]
	if len(sites) != 1 {
		t.Fatalf("func main must call %s exactly once, found %d", name, len(sites))
	}
	return sites[0]
}

// The cert volume has no other writer: the pki-agent sidecar is retired
// (ADR-000978), so the parent process is the only thing that mints the leaf
// the auth-hub introspection client presents.
func TestMain_StartsInProcessEnrollmentAsKnowledgeSovereign(t *testing.T) {
	calls := callsInMain(t)

	start := onlyCall(t, calls, "pki.Start")
	if got := stringArg(start.args, 2); got != "knowledge-sovereign" {
		t.Fatalf("pki.Start subject = %q, want %q (must match CERT_SUBJECT and the pki-agent-knowledge-sovereign provisioner)", got, "knowledge-sovereign")
	}

	ops := onlyCall(t, calls, "pki.ListenOps")
	if got := stringArg(ops.args, 2); got != "knowledge-sovereign" {
		t.Fatalf("pki.ListenOps service = %q, want %q", got, "knowledge-sovereign")
	}
	if ops.pos < start.pos {
		t.Fatal("pki.ListenOps must follow pki.Start so /metrics serves the enrollment registry")
	}
	onlyCall(t, calls, "pki.ShutdownOps")
}

// config.Load constructs the auth-hub mTLS verifier, which refuses to start
// without a leaf on disk. Enrollment therefore has to run before it, and
// before any listener accepts traffic.
func TestMain_EnrollsBeforeLoadingTheMTLSClient(t *testing.T) {
	calls := callsInMain(t)

	start := onlyCall(t, calls, "pki.Start")
	load := onlyCall(t, calls, "config.Load")
	if start.pos > load.pos {
		t.Fatal("pki.Start must run before config.Load (which loads the mTLS client cert)")
	}
	servers := onlyCall(t, calls, "startServers")
	if start.pos > servers.pos {
		t.Fatal("pki.Start must run before startServers")
	}
}

func TestMain_KeepsEnrollmentMetricsOffTheDefaultRegistry(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "prometheus.DefaultRegisterer") {
		t.Fatal("PKI enrollment metrics must stay on the private :9110 registry, not the :9501 default registry")
	}
}
