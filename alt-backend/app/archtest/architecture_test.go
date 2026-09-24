package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var portImportPattern = regexp.MustCompile(`^alt/.+/port(/|$)`)

var layerFamilies = []string{"orchestrator", "shared", "dataplane"}

var knownViolations = map[string]string{
	"shared/driver/alt_db":           "pending refactor: driver still returns domain types",
	"shared/driver/sovereign_client": "pending refactor: driver still returns domain types",
}

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

func getKnownViolation(relPath string) (string, string, bool) {
	for pkg, why := range knownViolations {
		if strings.HasPrefix(relPath, pkg+"/") || relPath == pkg {
			return pkg, why, true
		}
	}
	return "", "", false
}

// TestPortDoesNotImportNetHTTP verifies that port interfaces in all */port packages
// (orchestrator, shared, dataplane) do not import net/http.
func TestPortDoesNotImportNetHTTP(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	fset := token.NewFileSet()

	for _, family := range layerFamilies {
		targetDir := filepath.Join(moduleRoot, family, "port")
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			continue
		}

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
				if importPath == "net/http" {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports forbidden package %q: ports must be pure interfaces and must not import net/http", relPath, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s/port dir: %v", family, err)
		}
	}
}

// TestDriverDoesNotImplementPort verifies that drivers in all */driver packages
// (orchestrator, shared, dataplane) do not import port packages directly, skipping gateways.
func TestDriverDoesNotImplementPort(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	fset := token.NewFileSet()

	for _, family := range layerFamilies {
		targetDir := filepath.Join(moduleRoot, family, "driver")
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			continue
		}

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
				if portImportPattern.MatchString(importPath) {
					relPath, _ := filepath.Rel(moduleRoot, path)
					if pkg, why, ok := getKnownViolation(relPath); ok {
						t.Logf("KNOWN VIOLATION in %s (%s): %s imports port package %q", pkg, why, relPath, importPath)
						continue
					}
					t.Errorf("%s imports port package %q: drivers must not import ports directly (gateway must adapt driver to port)", relPath, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s/driver dir: %v", family, err)
		}
	}
}

// TestDriverDoesNotImportDomain verifies that drivers in all */driver packages
// (orchestrator, shared, dataplane) do not import domain models. Drivers must deal with raw rows/DTOs, and gateways map to domain.
func TestDriverDoesNotImportDomain(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	fset := token.NewFileSet()

	for _, family := range layerFamilies {
		targetDir := filepath.Join(moduleRoot, family, "driver")
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			continue
		}

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
				if importPath == "alt/domain" || strings.HasPrefix(importPath, "alt/domain/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					if pkg, why, ok := getKnownViolation(relPath); ok {
						t.Logf("KNOWN VIOLATION in %s (%s): %s imports domain package %q", pkg, why, relPath, importPath)
						continue
					}
					t.Errorf("%s imports domain package %q: drivers must not import domain models directly", relPath, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk %s/driver dir: %v", family, err)
		}
	}
}
