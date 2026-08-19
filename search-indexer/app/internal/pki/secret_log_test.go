package pki

import (
	"strings"
	"testing"
)

func assertNoPasswordFileInLogs(t *testing.T, logs string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		if strings.Contains(logs, s) {
			t.Fatalf("secret material %q leaked in logs: %s", s, logs)
		}
	}
}

func assertNoPasswordFileInError(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		if strings.Contains(msg, s) {
			t.Fatalf("secret material %q leaked in error: %v", s, err)
		}
	}
}
