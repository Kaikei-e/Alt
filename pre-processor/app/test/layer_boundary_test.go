package test

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

func parseFileImports(t *testing.T, filePath string) []string {
	t.Helper()
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("failed to parse file %s: %v", filePath, err)
	}
	var imports []string
	for _, imp := range fileNode.Imports {
		imports = append(imports, strings.Trim(imp.Path.Value, `"`))
	}
	return imports
}

func TestDriverMustNotImportDomain(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	driverRoot := filepath.Join(moduleRoot, "driver")

	err := filepath.WalkDir(driverRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		imports := parseFileImports(t, path)
		relPath, _ := filepath.Rel(moduleRoot, path)
		for _, imp := range imports {
			if imp == "pre-processor/domain" || strings.HasPrefix(imp, "pre-processor/domain/") {
				t.Errorf("%s imports forbidden package %q: driver layer must not import domain", relPath, imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk driver directory: %v", err)
	}
}

func TestHandlerMustNotImportPgx(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDirs := []string{
		filepath.Join(moduleRoot, "handler"),
		filepath.Join(moduleRoot, "connect"),
	}

	for _, dir := range targetDirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			imports := parseFileImports(t, path)
			relPath, _ := filepath.Rel(moduleRoot, path)
			for _, imp := range imports {
				if imp == "github.com/jackc/pgx/v5" || strings.HasPrefix(imp, "github.com/jackc/pgx/") {
					t.Errorf("%s imports forbidden package %q: handler/transport layer must not import pgx", relPath, imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk directory %s: %v", dir, err)
		}
	}
}
