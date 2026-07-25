package cmd

import (
	"bytes"
	"path/filepath"
	"testing"
)

// fixture returns the path to a checked-in fixture ADR directory.
func fixture(name string) string {
	return filepath.Join("..", "testdata", "fixtures", name)
}

// run executes a fresh root command with args and returns (stdout, stderr, exitCode).
func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	root := newRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errBuf.String(), exitCode(err)
}
