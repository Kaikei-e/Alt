//go:build parity

package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/graph"
)

const (
	pyScript = "../../scripts/adr_graph.py"
	pyCorpus = "../../docs/ADR"
)

// runPython invokes the canonical python tool and returns stdout, stderr, exit code.
func runPython(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("python3", append([]string{pyScript, "--adr-dir", pyCorpus}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Skipf("python3 unavailable: %v", err)
	}
	return out.String(), errBuf.String(), code
}

// TestParityCheck diffs `adrdag check` against `adr_graph.py check` on the
// live corpus: stdout, stderr, and exit code must be identical. This is the
// living contract that keeps "semantics-compatible with adr_graph.py"
// re-verified on every corpus change instead of hand-pinned.
func TestParityCheck(t *testing.T) {
	pyOut, pyErr, pyCode := runPython(t, "check")
	goOut, goErr, goCode := run(t, "--adr-dir", pyCorpus, "check")
	if goCode != pyCode {
		t.Errorf("exit: go=%d python=%d", goCode, pyCode)
	}
	if goOut != pyOut {
		t.Errorf("stdout diverges:\n--- go ---\n%s\n--- python ---\n%s", goOut, pyOut)
	}
	if goErr != pyErr {
		t.Errorf("stderr diverges:\n--- go ---\n%s\n--- python ---\n%s", goErr, pyErr)
	}
}

// TestParityResolve diffs resolve output for every ADR that has inbound
// supersedes edges (the ids where resolution is non-trivial), plus a terminal.
func TestParityResolve(t *testing.T) {
	adrs, err := adr.LoadDir(pyCorpus)
	if err != nil {
		t.Skipf("real corpus not available: %v", err)
	}
	reverse := graph.BuildReverse(adr.SupersedesGraph(adrs))
	ids := []string{}
	for _, id := range adr.SortedIDs(adrs) {
		if len(reverse[id]) > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		t.Fatal("no superseded ids found in corpus — parity test would be vacuous")
	}
	ids = append(ids, "000001") // a terminal: resolves to itself in both tools
	for _, id := range ids {
		pyOut, _, pyCode := runPython(t, "resolve", id)
		goOut, _, goCode := run(t, "--adr-dir", pyCorpus, "resolve", id)
		if goCode != pyCode || goOut != pyOut {
			t.Errorf("resolve %s: go=(%q,%d) python=(%q,%d)", id, goOut, goCode, pyOut, pyCode)
		}
	}
}

// TestParityGraph diffs the rendered mermaid projection byte-for-byte.
func TestParityGraph(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "py.md")
	goFile := filepath.Join(dir, "go.md")

	_, pyErr, pyCode := runPython(t, "graph", "--out", pyFile)
	if pyCode != 0 {
		t.Fatalf("python graph failed (%d): %s", pyCode, pyErr)
	}
	_, goErr, goCode := run(t, "--adr-dir", pyCorpus, "graph", "--out", goFile)
	if goCode != 0 {
		t.Fatalf("go graph failed (%d): %s", goCode, goErr)
	}
	pyBytes, err := os.ReadFile(pyFile)
	if err != nil {
		t.Fatal(err)
	}
	goBytes, err := os.ReadFile(goFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pyBytes, goBytes) {
		pyLines := strings.Split(string(pyBytes), "\n")
		goLines := strings.Split(string(goBytes), "\n")
		max := len(pyLines)
		if len(goLines) > max {
			max = len(goLines)
		}
		for i := 0; i < max; i++ {
			var p, g string
			if i < len(pyLines) {
				p = pyLines[i]
			}
			if i < len(goLines) {
				g = goLines[i]
			}
			if p != g {
				t.Errorf("mermaid line %d diverges:\npython: %q\ngo:     %q", i+1, p, g)
			}
		}
	}
}
