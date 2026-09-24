package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanArchitecture_Layers(t *testing.T) {
	root := "."
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "gen" || name == "vendor" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("failed to parse %s: %v", path, parseErr)
		}

		relPath, _ := filepath.Rel(root, path)
		slashPath := filepath.ToSlash(relPath)

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Rule: driver must not import domain, port, or upward layers
			if strings.HasPrefix(slashPath, "driver/") {
				if importPath == "search-indexer/domain" || strings.HasSuffix(importPath, "/domain") {
					t.Errorf("architecture violation: driver file %s imports domain (%s)", slashPath, importPath)
				}
				if importPath == "search-indexer/port" || strings.HasSuffix(importPath, "/port") {
					t.Errorf("architecture violation: driver file %s imports port (%s)", slashPath, importPath)
				}
				if strings.HasPrefix(importPath, "search-indexer/usecase") ||
					strings.HasPrefix(importPath, "search-indexer/gateway") ||
					strings.HasPrefix(importPath, "search-indexer/rest") ||
					strings.HasPrefix(importPath, "search-indexer/connect") {
					t.Errorf("architecture violation: driver file %s imports upward layer (%s)", slashPath, importPath)
				}
			}

			// Rule: port must not import driver, gateway, usecase, or rest/connect
			if strings.HasPrefix(slashPath, "port/") {
				if strings.HasPrefix(importPath, "search-indexer/driver") ||
					strings.HasPrefix(importPath, "search-indexer/gateway") ||
					strings.HasPrefix(importPath, "search-indexer/usecase") ||
					strings.HasPrefix(importPath, "search-indexer/rest") ||
					strings.HasPrefix(importPath, "search-indexer/connect") {
					t.Errorf("architecture violation: port file %s imports forbidden layer (%s)", slashPath, importPath)
				}
			}

			// Rule: usecase must not import driver, gateway, or rest/connect
			if strings.HasPrefix(slashPath, "usecase/") {
				if strings.HasPrefix(importPath, "search-indexer/driver") ||
					strings.HasPrefix(importPath, "search-indexer/gateway") ||
					strings.HasPrefix(importPath, "search-indexer/rest") ||
					strings.HasPrefix(importPath, "search-indexer/connect") {
					t.Errorf("architecture violation: usecase file %s imports forbidden layer (%s)", slashPath, importPath)
				}
			}

			// Rule: domain must not import sibling layers
			if strings.HasPrefix(slashPath, "domain/") {
				if strings.HasPrefix(importPath, "search-indexer/") {
					t.Errorf("architecture violation: domain file %s imports internal layer (%s)", slashPath, importPath)
				}
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("walking directory failed: %v", err)
	}
}
