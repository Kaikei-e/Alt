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
