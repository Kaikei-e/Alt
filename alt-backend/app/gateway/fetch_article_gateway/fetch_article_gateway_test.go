package fetch_article_gateway

import (
	"alt/domain"
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"context"
	"errors"
	"io"
	"net/http"
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

	content, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/article")
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

func TestFetchArticleGateway_NonSuccessStatus_ReturnsExternalHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"404 Not Found", 404},
		{"403 Forbidden", 403},
		{"410 Gone", 410},
		{"429 Too Many Requests", 429},
		{"500 Internal Server Error", 500},
		{"503 Service Unavailable", 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
			rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(strings.NewReader("error")),
					Header:     make(http.Header),
				}, nil
			})
			httpClient := &http.Client{Timeout: 2 * time.Second, Transport: rt}
			validator := security.NewSSRFValidator()
			gw := NewFetchArticleGatewayWithDeps(rl, httpClient, validator)

			_, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/article")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tt.statusCode)
			}

			var httpErr *domain.ExternalHTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("expected ExternalHTTPError, got %T: %v", err, err)
			}
			if httpErr.StatusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, httpErr.StatusCode)
			}
			if httpErr.URL == "" {
				t.Error("expected non-empty URL in ExternalHTTPError")
			}
		})
	}
}

// roundTripperFunc is a helper to stub http.RoundTripper
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
