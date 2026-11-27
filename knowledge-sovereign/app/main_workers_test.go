package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartWorkers_WiresTrailPlannerWithWallClock(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main_workers.go", nil, 0)
	require.NoError(t, err)

	var foundPlannerCall bool
	var hasClockField bool
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "trail_planner" || sel.Sel.Name != "NewPlanner" {
			return true
		}
		foundPlannerCall = true
		for _, arg := range call.Args {
			comp, ok := arg.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, elt := range comp.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Clock" {
					continue
				}
				valSel, ok := kv.Value.(*ast.SelectorExpr)
				if ok {
					valPkg, ok := valSel.X.(*ast.Ident)
					if ok && valPkg.Name == "time" && valSel.Sel.Name == "Now" {
						hasClockField = true
					}
				}
			}
		}
		return true
	})

	assert.True(t, foundPlannerCall, "expected trail_planner.NewPlanner call in main_workers.go")
	assert.True(t, hasClockField, "expected trail_planner.Config to wire Clock: time.Now")
}

// TestStartWorkers_LogsRule8WiringForAllThreeProjectors enforces Rule 8
// symmetry across knowledge_trail_projector, knowledge_home_projector, and
// reading_projector: every in-process projector must emit a loud startup wiring
// log naming its *_enabled state and repository_wired flag.
func TestStartWorkers_LogsRule8WiringForAllThreeProjectors(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main_workers.go", nil, 0)
	require.NoError(t, err)

	required := map[string]bool{
		`"trail.projector.wiring"`:   false,
		`"home.projector.wiring"`:    false,
		`"reading.projector.wiring"`: false,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 5 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "slog" || sel.Sel.Name != "Info" {
			return true
		}
		msg, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		if _, tracked := required[msg.Value]; !tracked {
			return true
		}
		var hasEnabled, hasRepoWired bool
		for _, arg := range call.Args[1:] {
			lit, ok := arg.(*ast.BasicLit)
			if !ok {
				continue
			}
			if lit.Value == `"enabled"` {
				hasEnabled = true
			}
			if lit.Value == `"repository_wired"` {
				hasRepoWired = true
			}
		}
		if hasEnabled && hasRepoWired {
			required[msg.Value] = true
		}
		return true
	})

	for msg, found := range required {
		assert.True(t, found, "expected startWorkers to emit Rule 8 wiring log %s with enabled and repository_wired fields", msg)
	}
}
