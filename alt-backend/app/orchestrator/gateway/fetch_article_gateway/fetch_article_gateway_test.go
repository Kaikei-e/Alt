package fetch_article_gateway

import (
	"alt/domain"
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
	var sawURL string
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		sawURL = req.URL.String()
		if req.URL.User != nil {
			t.Errorf("outbound request URL must not contain userinfo: %s", req.URL.Redacted())
		}
		if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
			t.Errorf("outbound request scheme %q is not allowlisted", req.URL.Scheme)
		}
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
	if sawURL != "https://93.184.216.34/article" {
		t.Fatalf("expected canonical outbound URL, got %q", sawURL)
	}
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

func TestFetchArticleGateway_UnreachableUpstream_ReturnsUpstreamFetchError(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{"deadline exceeded", context.DeadlineExceeded},
		{"dns lookup failed", &net.DNSError{Err: "no such host", Name: "www3.nhk.or.jp", IsNotFound: true}},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{"connection reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
			rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, tt.cause
			})
			httpClient := &http.Client{Timeout: 2 * time.Second, Transport: rt}
			gw := NewFetchArticleGatewayWithDeps(rl, httpClient, security.NewSSRFValidator())

			_, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/article")

			var upstreamErr *domain.UpstreamFetchError
			if !errors.As(err, &upstreamErr) {
				t.Fatalf("expected *domain.UpstreamFetchError, got %T: %v", err, err)
			}
			if upstreamErr.URL == "" {
				t.Error("expected the fetched URL to be carried on the error")
			}
			if !errors.Is(err, tt.cause) {
				t.Errorf("expected the transport cause to stay unwrappable, got: %v", err)
			}
		})
	}
}

// okTransport answers every request with a short HTML body.
func okTransport() roundTripperFunc {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("<p>OK</p>")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}, nil
	}
}

func TestFetchArticleGateway_HostSlotWaitExpires_ReturnsRateLimitedError(t *testing.T) {
	// One token, then a ten-second refill: the second caller's turn at the host
	// never comes. Nothing is sent, so this is our own politeness gate working
	// as designed — a transient "come back shortly", not a failed dependency
	// and not something the publisher did.
	rl := rate_limiter.NewHostRateLimiter(10 * time.Second)
	httpClient := &http.Client{Timeout: 2 * time.Second, Transport: okTransport()}
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, security.NewSSRFValidator())

	if _, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/first"); err != nil {
		t.Fatalf("first fetch should consume the host's only token: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := gw.FetchArticleContents(ctx, "https://93.184.216.34/second")

	var rateErr *domain.RateLimitedError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected *domain.RateLimitedError, got %T: %v", err, err)
	}
	if rateErr.RetryAfter <= 0 {
		t.Errorf("expected an actionable Retry-After, got %v", rateErr.RetryAfter)
	}

	var upstreamErr *domain.UpstreamFetchError
	if errors.As(err, &upstreamErr) {
		t.Fatalf("our own queue must not be reported as the publisher failing: %v", err)
	}
}

func TestFetchArticleGateway_InteractiveSlotBudget_GivesUpTheTurnPromptly(t *testing.T) {
	// A caller with a user waiting bounds its own wait. Without the budget this
	// call would sit on the ten-second refill; with it, it gives the turn up
	// and hands the client something to act on.
	rl := rate_limiter.NewHostRateLimiter(10 * time.Second)
	httpClient := &http.Client{Timeout: 2 * time.Second, Transport: okTransport()}
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, security.NewSSRFValidator())
	gw.slotWaitBudget = 50 * time.Millisecond

	if _, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/first"); err != nil {
		t.Fatalf("first fetch should consume the host's only token: %v", err)
	}

	start := time.Now()
	_, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/second")
	elapsed := time.Since(start)

	var rateErr *domain.RateLimitedError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected *domain.RateLimitedError, got %T: %v", err, err)
	}
	if rateErr.RetryAfter <= 0 {
		t.Errorf("expected an actionable Retry-After, got %v", rateErr.RetryAfter)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected the caller to give up promptly, waited %v", elapsed)
	}
}

func TestFetchArticleGateway_BackgroundCaller_WaitsForItsTurn(t *testing.T) {
	// No budget: a job with nobody waiting keeps the turn it queued for, which
	// is what keeps the per-host interval a promise rather than a preference.
	const interval = 300 * time.Millisecond
	rl := rate_limiter.NewHostRateLimiter(interval)
	httpClient := &http.Client{Timeout: 2 * time.Second, Transport: okTransport()}
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, security.NewSSRFValidator())

	if _, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/first"); err != nil {
		t.Fatalf("first fetch should consume the host's only token: %v", err)
	}

	start := time.Now()
	content, err := gw.FetchArticleContents(context.Background(), "https://93.184.216.34/second")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected the background caller to wait for its turn and succeed, got %v", err)
	}
	if content == nil || *content == "" {
		t.Fatal("expected content once the turn came")
	}
	if elapsed < interval-50*time.Millisecond {
		t.Errorf("expected the caller to wait out the host interval, returned after %v", elapsed)
	}
}

func TestNewInteractiveFetchArticleGateway_CarriesTheSlotWaitBudget(t *testing.T) {
	gw := NewInteractiveFetchArticleGateway(
		rate_limiter.NewHostRateLimiter(1*time.Millisecond),
		&http.Client{Timeout: 2 * time.Second},
		3*time.Second,
	)

	if gw.slotWaitBudget != 3*time.Second {
		t.Errorf("expected the interactive gateway to bound its slot wait, got %v", gw.slotWaitBudget)
	}
}

func TestFetchArticleGateway_SSRFBlocked_IsNotUpstreamFetchError(t *testing.T) {
	// A refused address is a decision of ours, not a publisher that went quiet.
	// Keeping it unclassified is what keeps it an internal fault.
	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	gw := NewFetchArticleGateway(rl, &http.Client{Timeout: 2 * time.Second})

	_, err := gw.FetchArticleContents(context.Background(), "http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected error for metadata endpoint, got nil")
	}

	var upstreamErr *domain.UpstreamFetchError
	if errors.As(err, &upstreamErr) {
		t.Fatalf("SSRF refusal must not be reported as an upstream fetch failure: %v", err)
	}
}

func TestFetchArticleGateway_Semaphore_LimitsConcurrency(t *testing.T) {
	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)

	// Track concurrent in-flight requests
	var inflight int32
	var maxInflight int32

	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		cur := atomic.AddInt32(&inflight, 1)
		defer atomic.AddInt32(&inflight, -1)

		// Update max observed concurrency
		for {
			old := atomic.LoadInt32(&maxInflight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInflight, old, cur) {
				break
			}
		}

		// Simulate slow external fetch
		time.Sleep(50 * time.Millisecond)

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("<p>OK</p>")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}, nil
	})

	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: rt}
	validator := security.NewSSRFValidator()
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, validator)

	// Launch 6 concurrent fetches (semaphore limit = 3)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = gw.FetchArticleContents(context.Background(), fmt.Sprintf("https://93.184.216.%d/article", idx+1))
		}(i)
	}
	wg.Wait()

	observed := atomic.LoadInt32(&maxInflight)
	if observed > 3 {
		t.Errorf("expected max 3 concurrent fetches, observed %d", observed)
	}
}

func TestFetchArticleGateway_Semaphore_ContextCancellation(t *testing.T) {
	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)

	// Slow transport that holds semaphore slots
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		time.Sleep(2 * time.Second)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("<p>OK</p>")),
			Header:     http.Header{"Content-Type": []string{"text/html"}},
		}, nil
	})

	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: rt}
	validator := security.NewSSRFValidator()
	gw := NewFetchArticleGatewayWithDeps(rl, httpClient, validator)

	// Fill all 3 semaphore slots
	for i := 0; i < 3; i++ {
		go func(idx int) {
			_, _ = gw.FetchArticleContents(context.Background(), fmt.Sprintf("https://93.184.216.%d/article", idx+1))
		}(i)
	}
	time.Sleep(10 * time.Millisecond) // Let goroutines acquire slots

	// Now try with a context that expires quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := gw.FetchArticleContents(ctx, "https://93.184.216.100/article")
	if err == nil {
		t.Fatal("expected error when context expires waiting for semaphore")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}

// gzipBomb returns a gzip stream that is tiny on the wire but expands to
// decompressedSize bytes of fill. Go's http.Transport auto-adds
// "Accept-Encoding: gzip" and transparently decompresses, so this is what an
// attacker-controlled article URL can hand the gateway.
func gzipBomb(t *testing.T, fill byte, decompressedSize int) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	chunk := bytes.Repeat([]byte{fill}, 64*1024)
	for written := 0; written < decompressedSize; written += len(chunk) {
		if _, err := zw.Write(chunk); err != nil {
			t.Fatalf("write gzip chunk failed: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close gzip writer failed: %v", err)
	}
	return buf.Bytes()
}

func TestFetchArticleGateway_OversizedBody_Rejected(t *testing.T) {
	bomb := gzipBomb(t, 'A', maxArticleBodyBytes*4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	defer server.Close()

	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	validator := security.NewSSRFValidator()
	validator.SetTestingMode(true)
	gw := NewFetchArticleGatewayWithDeps(rl, &http.Client{Timeout: 30 * time.Second}, validator)

	content, err := gw.FetchArticleContents(context.Background(), server.URL+"/article")
	if err == nil {
		t.Fatalf("expected error for oversized body, got content of %d bytes", len(*content))
	}
	if content != nil {
		t.Errorf("expected nil content on oversized body, got %d bytes", len(*content))
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size-limit error, got: %v", err)
	}
}

// shiftJISFill is a halfwidth katakana byte (U+FF61 "｡"). Shift-JIS encodes it in
// one byte; UTF-8 needs three. Bounding only the pre-decode stream therefore lets
// a body that fits under the ceiling triple past it inside the decoder.
const shiftJISFill = 0xA1

func TestFetchArticleGateway_OversizedDecodedBody_Rejected(t *testing.T) {
	// Half the ceiling on the wire, so the pre-decode budget is never exhausted —
	// only the 3x charset expansion pushes the result over the limit.
	bomb := gzipBomb(t, shiftJISFill, maxArticleBodyBytes/2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=shift_jis")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(bomb)
	}))
	defer server.Close()

	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	validator := security.NewSSRFValidator()
	validator.SetTestingMode(true)
	gw := NewFetchArticleGatewayWithDeps(rl, &http.Client{Timeout: 30 * time.Second}, validator)

	content, err := gw.FetchArticleContents(context.Background(), server.URL+"/article")
	if err == nil {
		t.Fatalf("expected error for oversized decoded body, got content of %d bytes", len(*content))
	}
	if content != nil {
		t.Errorf("expected nil content on oversized decoded body, got %d bytes", len(*content))
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size-limit error, got: %v", err)
	}
}

func TestFetchArticleGateway_ShiftJISBody_DecodedInFull(t *testing.T) {
	// Control for the ceiling above: a legitimate Shift-JIS article still comes
	// back fully transcoded to UTF-8.
	article := strings.Repeat("\xa1", 1000)
	want := strings.Repeat("｡", 1000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=shift_jis")
		_, _ = io.WriteString(w, article)
	}))
	defer server.Close()

	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	validator := security.NewSSRFValidator()
	validator.SetTestingMode(true)
	gw := NewFetchArticleGatewayWithDeps(rl, &http.Client{Timeout: 30 * time.Second}, validator)

	content, err := gw.FetchArticleContents(context.Background(), server.URL+"/article")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if content == nil || *content != want {
		t.Fatalf("expected the Shift-JIS article to be decoded in full")
	}
}

func TestFetchArticleGateway_NormalBody_FetchedInFull(t *testing.T) {
	article := "<html><body><p>" + strings.Repeat("normal article text. ", 1000) + "</p></body></html>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, article)
	}))
	defer server.Close()

	rl := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	validator := security.NewSSRFValidator()
	validator.SetTestingMode(true)
	gw := NewFetchArticleGatewayWithDeps(rl, &http.Client{Timeout: 30 * time.Second}, validator)

	content, err := gw.FetchArticleContents(context.Background(), server.URL+"/article")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if content == nil || *content != article {
		t.Fatalf("expected the full article body to be returned unchanged")
	}
}

// roundTripperFunc is a helper to stub http.RoundTripper
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
