package fetch_article_gateway

import (
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchArticleGateway_SSRF_Blocked(t *testing.T) {
	rl := rate_limiter.NewHostRateLimiter(10 * time.Millisecond)
	client := &http.Client{Timeout: 2 * time.Second}
	gw := NewFetchArticleGateway(rl, client)

	// Metadata endpoint should be blocked
	_, err := gw.FetchArticleContents(context.Background(), "http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatalf("expected error for metadata endpoint, got nil")
	}

	// Localhost should be blocked by default
	_, err = gw.FetchArticleContents(context.Background(), "http://127.0.0.1:8080/")
	if err == nil {
		t.Fatalf("expected error for localhost, got nil")
	}
}

func TestFetchArticleGateway_Fetch_Success_WithTestingOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><h1>Hello</h1><p>World</p></body></html>"))
	}))
	defer server.Close()

	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)

	// Fake RoundTripper to avoid real network
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("<h1>OK</h1>")),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Type", "text/html")
		return resp, nil
	})
	httpClient := &http.Client{Timeout: 2 * time.Second, Transport: rt}
	validator := security.NewSSRFValidator()
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, validator)

	content, err := gw.FetchArticleContents(context.Background(), "https://example.com/article")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if content == nil || *content == "" {
		t.Fatalf("expected non-empty content, got %v", content)
	}
	// HTML should be stripped roughly (we don't assert exact value here)
}

func TestFetchArticleGateway_InvalidURL(t *testing.T) {
	rl := rate_limiter.NewHostRateLimiter(10 * time.Millisecond)
	client := &http.Client{Timeout: 2 * time.Second}
	gw := NewFetchArticleGateway(rl, client)

	_, err := gw.FetchArticleContents(context.Background(), "://bad-url")
	if err == nil {
		t.Fatalf("expected error for invalid URL, got nil")
	}
}

// roundTripperFunc is a helper to stub http.RoundTripper
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
