package pki

import (
	"bytes"
	"strings"
	"sync"
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

// syncBuffer captures slog output that a test polls while the code under test
// is still writing to it from another goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
