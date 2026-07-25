package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/adrdag/internal/adr"
	"github.com/alt-project/adrdag/internal/render"
)

func TestGraphMermaidToStdout(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "graph")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	adrs, err := adr.LoadDir(fixture("ok"))
	if err != nil {
		t.Fatal(err)
	}
	want := render.Mermaid(adr.SupersedesGraph(adrs), adrs) + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if !strings.HasPrefix(stdout, "```mermaid\ngraph LR\n") {
		t.Errorf("stdout does not start with mermaid fence: %q", stdout)
	}
}

func TestGraphDOT(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "graph", "--format", "dot")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if !strings.HasPrefix(stdout, "digraph") {
		t.Errorf("stdout does not start with digraph: %q", stdout)
	}
}

func TestGraphJSON(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "graph", "--format", "json")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, `"links"`) {
		t.Errorf("stdout missing links key: %q", stdout)
	}
}

func TestGraphOutWritesFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "graph.md")
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "graph", "--out", out)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	// parity with adr_graph.py's cmd_graph: print "wrote <path>"
	if want := "wrote " + out + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	adrs, err := adr.LoadDir(fixture("ok"))
	if err != nil {
		t.Fatal(err)
	}
	if want := render.Mermaid(adr.SupersedesGraph(adrs), adrs) + "\n"; string(content) != want {
		t.Errorf("file = %q, want %q", content, want)
	}
}

func TestGraphBadFormatIsUsageError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("ok"), "graph", "--format", "png")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}

func TestGraphUnwritableOutIsIOError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("ok"), "graph", "--out", filepath.Join(string(os.PathSeparator), "proc", "no", "such", "dir", "x.md"))
	if code != exitIO {
		t.Fatalf("exit = %d, want %d", code, exitIO)
	}
}
