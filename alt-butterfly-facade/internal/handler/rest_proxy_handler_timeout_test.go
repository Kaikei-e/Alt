package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"alt-butterfly-facade/internal/client"
)

// TestRESTProxyHandler_AppliesRequestTimeout pins the deadline on the REST
// proxy path. The backend client deliberately runs with http.Client.Timeout
// unset, so if ServeHTTP does not put a deadline on the request context an
// alt-backend that accepts the connection but never answers keeps this
// handler goroutine — and its client connection — alive forever.
// WriteTimeout on the server only hangs up on the caller; it never unblocks
// the handler.
func TestRESTProxyHandler_AppliesRequestTimeout(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept the request and never write a response.
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer backend.Close()
	defer close(release)

	backendClient := client.NewBackendClientWithTransport(
		backend.URL,
		100*time.Millisecond,
		100*time.Millisecond,
		http.DefaultTransport,
	)
	handler := NewRESTProxyHandler(
		backendClient,
		[]byte(testRESTSecret),
		testRESTIssuer,
		testRESTAudience,
		nil,
		100*time.Millisecond,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set("X-Alt-Backend-Token", createTestJWT(t))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP never returned: requestTimeout is stored but not applied, so a silent backend pins the handler goroutine indefinitely")
	}

	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"an upstream that blows the request deadline must surface as 502, not as a hung goroutine")
}
