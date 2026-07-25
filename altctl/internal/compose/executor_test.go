package compose

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestRunWithPipes_WiresStdin guards against the exec/pipe path silently
// dropping stdin: `altctl exec db -- psql` needs an interactive/piped stdin
// wired through to the child process, or the child sees instant EOF.
func TestRunWithPipes_WiresStdin(t *testing.T) {
	e := NewExecutor(t.TempDir(), slog.New(slog.NewTextHandler(os.Stderr, nil)), false)
	e.SetStdin(strings.NewReader("hello from stdin\n"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// `cat` with no args echoes stdin to stdout and exits when stdin is
	// closed/EOF -- if Stdin isn't wired, cat gets an immediate EOF (from
	// /dev/null-equivalent) and stdout stays empty.
	err := e.RunWithPipes(context.Background(), "cat", nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithPipes failed: %v (stderr: %s)", err, stderr.String())
	}

	if got := stdout.String(); got != "hello from stdin\n" {
		t.Errorf("stdout = %q, want %q -- stdin was not wired to the child process", got, "hello from stdin\n")
	}
}

// TestNewExecutor_DefaultsStdinToOSStdin documents the production default:
// real invocations (e.g. `altctl exec db -- psql`) must see the operator's
// terminal/piped stdin, not an implicit EOF.
func TestNewExecutor_DefaultsStdinToOSStdin(t *testing.T) {
	e := NewExecutor(t.TempDir(), slog.Default(), false)
	if e.stdin != os.Stdin {
		t.Error("NewExecutor should default stdin to os.Stdin")
	}
}
