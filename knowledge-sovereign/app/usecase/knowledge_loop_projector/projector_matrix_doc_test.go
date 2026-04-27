package knowledge_loop_projector

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProjectorMatrix_DocCovered binds the human-readable matrix at
// docs/features/knowledge-loop/projector-matrix.md to the event-type
// constants in this package. Drift fails CI in either direction:
//
//	(a) a new Event* constant in constants.go must add a row to the matrix
//	(b) a row in the matrix must reference an existing Event* constant value
//
// This is the cheap, durable replacement for "we updated constants.go but
// forgot to update the doc". It runs without a database.
//
// The doc location is stable; if you move it, update both this test and any
// link in CLAUDE.md / README.md so the binding stays explicit.
const projectorMatrixDocRelPath = "../../../../docs/features/knowledge-loop/projector-matrix.md"

func TestProjectorMatrix_DocCovered(t *testing.T) {
	t.Parallel()

	docEvents := parseMatrixEventTypes(t)
	codeEvents := parseEventConstantValues(t)

	// Every doc row must correspond to a real Event* constant.
	for _, ev := range docEvents {
		require.Containsf(t, codeEvents, ev,
			"projector-matrix.md row %q has no matching Event* constant in constants.go", ev)
	}

	// Every Event* constant must appear in the doc, so adding a new event
	// type without documenting its projector behaviour fails CI.
	for _, ev := range codeEvents {
		require.Containsf(t, docEvents, ev,
			"constants.go declares %q but projector-matrix.md does not document it — add a row before merging", ev)
	}
}

// parseMatrixEventTypes extracts the first column (Event type) of the matrix
// table in projector-matrix.md. Header rows and separator rows are skipped.
func parseMatrixEventTypes(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(projectorMatrixDocRelPath))
	require.NoErrorf(t, err, "read projector-matrix.md")
	got := []string{}
	inMatrix := false
	for line := range strings.SplitSeq(string(body), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## Matrix") {
			inMatrix = true
			continue
		}
		if inMatrix && strings.HasPrefix(trim, "## ") {
			break
		}
		if !inMatrix || !strings.HasPrefix(trim, "|") {
			continue
		}
		// Skip the header row ("| Event type | ...") and the separator row
		// ("|---|---|...").
		if strings.Contains(trim, "Event type") {
			continue
		}
		if strings.Count(trim, "|") > 0 && strings.ReplaceAll(trim, "|", "") != "" {
			cleaned := strings.ReplaceAll(trim, "-", "")
			if strings.TrimSpace(strings.ReplaceAll(cleaned, "|", "")) == "" {
				continue
			}
		}
		// First column.
		cells := strings.SplitN(trim, "|", 3)
		if len(cells) < 2 {
			continue
		}
		first := strings.TrimSpace(cells[1])
		if first == "" {
			continue
		}
		got = append(got, first)
	}
	require.NotEmpty(t, got, "projector-matrix.md table appears to be empty — parser bug or doc deletion")
	return got
}

// parseEventConstantValues extracts the *value* (the literal string) of every
// `Event*` constant declared in constants.go. We use go/ast so a stale text
// search can't lie about the literal.
func parseEventConstantValues(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "constants.go", nil, parser.AllErrors)
	require.NoErrorf(t, err, "parse constants.go")

	values := []string{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				// Trim surrounding quotes.
				v := strings.Trim(lit.Value, `"`)
				values = append(values, v)
			}
		}
	}
	require.NotEmpty(t, values, "no Event* constants extracted from constants.go — parser bug")
	return values
}

// TestProjectorMatrix_NoStaleEntries flags rows that look like Event* keys but
// don't follow the project's event_type vocabulary. The vocabulary is mixed:
// Phase-0 events (PascalCase) and Loop events (snake_case.v1). Anything else
// is almost certainly a typo; the doc + the consts must agree literally.
func TestProjectorMatrix_NoStaleEntries(t *testing.T) {
	t.Parallel()
	pascal := regexp.MustCompile(`^[A-Z][A-Za-z0-9]+$`)
	versioned := regexp.MustCompile(`^[a-z][a-z0-9_.]+\.v[0-9]+$`)
	for _, ev := range parseMatrixEventTypes(t) {
		require.Truef(t, pascal.MatchString(ev) || versioned.MatchString(ev),
			"projector-matrix.md row %q does not match the Phase-0 (PascalCase) or Loop (snake_case.v1) event vocabulary", ev)
	}
}
