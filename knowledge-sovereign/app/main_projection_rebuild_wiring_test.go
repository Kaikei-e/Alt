package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/handler"
)

type stubRebuildRepo struct {
	calls int
}

func (s *stubRebuildRepo) RebuildProjection(context.Context, sovereign_db.ProjectionRebuildTarget) (sovereign_db.ProjectionRebuildResult, error) {
	s.calls++
	return sovereign_db.ProjectionRebuildResult{Target: "knowledge-home"}, nil
}

// The rebuild endpoint truncates read models. It must sit behind the same admin
// token gate as the snapshot writer and the retention exporter — an
// unauthenticated caller must not be able to empty a projection.
func TestProjectionRebuild_IsBehindTheAdminToken(t *testing.T) {
	repo := &stubRebuildRepo{}
	mux := http.NewServeMux()
	handler.NewProjectionRebuildHandler(repo).RegisterRoutes(mux)
	gated := requireAdminToken("super-secret-admin-token-value", true, mux)

	unauth := httptest.NewRequest(http.MethodPost, "/admin/projections/rebuild",
		strings.NewReader(`{"target":"knowledge-home"}`))
	rec := httptest.NewRecorder()
	gated.ServeHTTP(rec, unauth)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unauthenticated rebuild, got %d", rec.Code)
	}
	if repo.calls != 0 {
		t.Fatalf("an unauthenticated rebuild reached the database %d time(s)", repo.calls)
	}

	authed := httptest.NewRequest(http.MethodPost, "/admin/projections/rebuild",
		strings.NewReader(`{"target":"knowledge-home"}`))
	authed.Header.Set("Authorization", "Bearer super-secret-admin-token-value")
	rec = httptest.NewRecorder()
	gated.ServeHTTP(rec, authed)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an authenticated rebuild, got %d (%s)", rec.Code, rec.Body.String())
	}
	if repo.calls != 1 {
		t.Fatalf("expected exactly one rebuild, got %d", repo.calls)
	}
}

// A handler nobody registers is the same defect the partition maintainer had:
// green unit tests over an operation no operator can ever invoke. Assert
// against main.go's AST that the rebuild routes are actually mounted on the
// metrics mux.
func TestMain_WiresProjectionRebuildIntoAdminSurface(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("main.go has no func main")
	}

	var constructs, registers bool
	ast.Inspect(mainFunc.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "handler" && sel.Sel.Name == "NewProjectionRebuildHandler" {
			constructs = true
		}
		if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "projectionRebuildHandler" && sel.Sel.Name == "RegisterRoutes" {
			if len(call.Args) == 1 {
				if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == "metricsMux" {
					registers = true
				}
			}
		}
		return true
	})

	if !constructs {
		t.Error("main() must construct handler.NewProjectionRebuildHandler")
	}
	if !registers {
		t.Error("main() must call projectionRebuildHandler.RegisterRoutes(metricsMux) — an unregistered rebuild handler is unreachable")
	}
}
