package backend_api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClient_HTTPClient_HasTimeout verifies the Connect-RPC client uses an
// http.Client with a non-zero Timeout, protecting search-indexer from
// alt-backend hangs (ADR-000717 / C-003).
func TestClient_HTTPClient_HasTimeout(t *testing.T) {
	t.Parallel()

	hc := DefaultHTTPClient()
	if hc.Timeout == 0 {
		t.Fatal("http.Client.Timeout must be non-zero to prevent resource exhaustion")
	}
	if hc.Timeout > 60*time.Second {
		t.Fatalf("http.Client.Timeout=%s is too permissive (want <= 60s)", hc.Timeout)
	}
	if hc == http.DefaultClient {
		t.Fatal("must not reuse http.DefaultClient; construct a dedicated Client")
	}
}

// TestClient_TimesOutOnSlowBackend verifies that a hanging backend server
// causes the client to return within the configured timeout rather than
// blocking forever.
func TestClient_TimesOutOnSlowBackend(t *testing.T) {
	t.Parallel()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block longer than any sane timeout.
		time.Sleep(5 * time.Second)
	}))
	defer slow.Close()

	c := NewClient(slow.URL, "test-token", nil)

	// Bound the call with a short context so the test completes quickly even
	// if the http.Client timeout regresses.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, _, err := c.GetArticlesWithTags(ctx, nil, "", 10)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error from slow backend, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("call took %s; expected fast timeout via context/client", elapsed)
	}
}
