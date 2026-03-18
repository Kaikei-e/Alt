package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestHomeSLOCmd_Exists(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "slo", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home slo --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "SLO") {
		t.Errorf("expected help to mention 'SLO', got:\n%s", out)
	}
}

func TestHomeSLOCmd_Usage(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "slo", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home slo --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "slo") {
		t.Errorf("expected usage to contain 'slo', got:\n%s", out)
	}
}

func TestHomeSLOCmd_AcceptsBackendURL(t *testing.T) {
	setupHomeTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"home", "slo", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("home slo --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "backend-url") {
		t.Errorf("expected help to mention 'backend-url' flag, got:\n%s", out)
	}
}
