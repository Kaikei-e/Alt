package httpclient

import (
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewPooledClient_FailedMTLSLoadRefusesPlaintext is the RED case for the
// bug: when MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE fail to
// load, NewPooledClient used to return a client with no Transport, which
// falls back to http.DefaultTransport — i.e. it would happily complete a
// plaintext request against an http:// target instead of failing closed.
// This exercises the exact gap the review flagged: cmd/backfill's --direct
// mode calls NewPooledClient without ever calling PreflightMTLS first, so
// this fallback was the only thing standing between "cert load failed" and
// "request goes out in plaintext anyway".
func TestNewPooledClient_FailedMTLSLoadRefusesPlaintext(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")

	srv := httptest.NewServer(nil)
	defer srv.Close()

	c := NewPooledClient(2 * time.Second)
	resp, err := c.Get(srv.URL)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("expected request to fail closed when mTLS cert loading failed, but it succeeded in plaintext")
	}
}
