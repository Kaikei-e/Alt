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

// TestRESTHandlersDoNotImportGateways verifies that REST handler packages
// (internal/adapter/connect/... and internal/adapter/rag_http) do not import
// gateway adapter packages or direct infra/SQL drivers.
func TestRESTHandlersDoNotImportGateways(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	handlerDirs := []string{
		filepath.Join(moduleRoot, "internal", "adapter", "connect"),
		filepath.Join(moduleRoot, "internal", "adapter", "rag_http"),
	}

	fset := token.NewFileSet()
	scannedFiles := 0

	for _, targetDir := range handlerDirs {
		if _, err := os.Stat(targetDir); err != nil {
			t.Fatalf("handler dir %s does not exist: %v", targetDir, err)
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
			scannedFiles++

			for _, imp := range fileNode.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)

				// Forbid raw SQL drivers in REST layer.
				if importPath == "database/sql" || strings.HasPrefix(importPath, "github.com/jackc/pgx") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports raw database package %q: REST handlers must not import SQL drivers", relPath, importPath)
				}

				// Forbid imports with prefix rag-orchestrator/internal/adapter/,
				// except adapter/connect/* itself, adapter/rag_http/* itself, and adapter/contract.
				if strings.HasPrefix(importPath, "rag-orchestrator/internal/adapter/") {
					if strings.HasPrefix(importPath, "rag-orchestrator/internal/adapter/connect") ||
						strings.HasPrefix(importPath, "rag-orchestrator/internal/adapter/rag_http") ||
						strings.HasPrefix(importPath, "rag-orchestrator/internal/adapter/contract") {
						continue
					}
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports gateway adapter %q: REST handlers must not import gateway adapters directly", relPath, importPath)
				}

				// Check infra imports: only metrics is allowed as cross-cutting telemetry.
				if strings.HasPrefix(importPath, "rag-orchestrator/internal/infra") {
					// internal/infra/metrics is allowed in REST handlers as cross-cutting telemetry for request and stream observability.
					if importPath == "rag-orchestrator/internal/infra/metrics" {
						continue
					}
					// search_indexer_client.go is a gateway client colocated in the rag_http directory.
					if strings.HasSuffix(path, "search_indexer_client.go") {
						continue
					}
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports infra package %q: REST handlers must not import infra directly", relPath, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk handler dir %s: %v", targetDir, err)
		}
	}

	if scannedFiles == 0 {
		t.Fatal("expected to scan at least one handler file, found none")
	}
}

// TestUsecaseLayerBoundaries verifies that usecase packages do not import
// gateway adapters, drivers, REST handlers, or raw SQL/HTTP libraries.
func TestUsecaseLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "internal", "usecase")

	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("usecase dir %s does not exist: %v", targetDir, err)
	}

	forbidden := []string{
		"rag-orchestrator/internal/adapter",
		"rag-orchestrator/internal/infra",
		"github.com/jackc/pgx",
		"database/sql",
		"net/http",
	}

	fset := token.NewFileSet()
	scannedFiles := 0
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
		scannedFiles++

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports forbidden package %q: usecases must not import gateway, infra, or raw I/O", relPath, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk usecase dir: %v", err)
	}

	if scannedFiles == 0 {
		t.Fatal("expected to scan at least one usecase file, found none")
	}
}

// TestDriverLayerBoundaries verifies that drivers in internal/infra do not
// import domain models or upward layers (usecase, adapter).
func TestDriverLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "internal", "infra")

	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("infra dir %s does not exist: %v", targetDir, err)
	}

	forbidden := []string{
		"rag-orchestrator/internal/domain",
		"rag-orchestrator/internal/usecase",
		"rag-orchestrator/internal/adapter",
	}

	fset := token.NewFileSet()
	scannedFiles := 0
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
		scannedFiles++

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, path)
					t.Errorf("%s imports forbidden package %q: driver layer must not import domain or upward layers", relPath, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk infra dir: %v", err)
	}

	if scannedFiles == 0 {
		t.Fatal("expected to scan at least one infra file, found none")
	}
}

// TestDomainLayerBoundaries verifies that internal/domain does not import other internal layers.
func TestDomainLayerBoundaries(t *testing.T) {
	moduleRoot := findModuleRoot(t)
	targetDir := filepath.Join(moduleRoot, "internal", "domain")

	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("domain dir %s does not exist: %v", targetDir, err)
	}

	forbidden := []string{
		"rag-orchestrator/internal/usecase",
		"rag-orchestrator/internal/adapter",
		"rag-orchestrator/internal/infra",
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatalf("failed to read domain dir: %v", err)
	}

	fset := token.NewFileSet()
	scannedFiles := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(targetDir, entry.Name())
		fileNode, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("failed to parse file %s: %v", filePath, err)
		}
		scannedFiles++

		for _, imp := range fileNode.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forb := range forbidden {
				if importPath == forb || strings.HasPrefix(importPath, forb+"/") {
					relPath, _ := filepath.Rel(moduleRoot, filePath)
					t.Errorf("%s imports forbidden package %q: domain must not import other internal layers", relPath, importPath)
				}
			}
		}
	}

	if scannedFiles == 0 {
		t.Fatal("expected to scan at least one domain file, found none")
	}
}
