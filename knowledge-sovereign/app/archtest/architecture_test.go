package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod in parents")
		}
		dir = parent
	}
}

// TestHandlerLayerBoundaries verifies that handler packages adhere to the documented reduced shape:
// Handlers may import driver/sovereign_db, but must not import raw database drivers directly (e.g. pgx, database/sql).
func TestHandlerLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "handler")

	forbidden := []string{
		"github.com/jackc/pgx",
		"database/sql",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fileNode, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", path, err)
		}

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports raw database package %q: handlers must use driver/sovereign_db instead of importing raw database drivers directly", relPath, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk handler dir: %v", err)
	}
}

// TestUsecaseLayerBoundaries verifies that usecase packages adhere to the documented reduced shape:
// Usecases may import driver/sovereign_db, but must not import handler or raw database drivers directly.
func TestUsecaseLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "usecase")

	forbidden := []string{
		"knowledge-sovereign/handler",
		"github.com/jackc/pgx",
		"database/sql",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fileNode, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", path, err)
		}

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports forbidden package %q: usecases must not import handler or raw database drivers", relPath, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk usecase dir: %v", err)
	}
}

// TestDriverLayerBoundaries verifies that production drivers do not import upward layers (handler, usecase).
// driver/contract is excluded as it is a Pact CDC provider test harness under //go:build contract.
func TestDriverLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "driver")

	forbidden := []string{
		"knowledge-sovereign/handler",
		"knowledge-sovereign/usecase",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "contract" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fileNode, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", path, err)
		}

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports forbidden package %q: drivers must not import handler or usecase", relPath, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk driver dir: %v", err)
	}
}
