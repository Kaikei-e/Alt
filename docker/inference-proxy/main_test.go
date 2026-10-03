package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// TestProxy_StripAuth_And_Forwarding verifies:
//   - /health is unauthenticated and never hits upstream
//   - missing / bad auth returns 401 and hits upstream 0 times
//   - valid auth forwards the request, strips Authorization, and hits upstream exactly once
func TestProxy_StripAuth_And_Forwarding(t *testing.T) {
	expectedToken := "test-secret-token"
	var upstreamHits int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Upstream should not receive Authorization header, got: %s", auth)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer upstream.Close()

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Parse url failed: %v", err)
	}

	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	// 1. /health — unauthenticated, upstream hit count must stay at 0.
	resp, err := http.Get(proxyServer.URL + "/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for health, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Errorf("Health endpoint must not hit upstream, got %d hit(s)", h)
	}

	// 2. Missing auth — upstream must not be hit.
	resp, err = http.Get(proxyServer.URL + "/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for missing auth, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Errorf("Missing-auth request must not hit upstream, got %d hit(s)", h)
	}

	// 3. Bad auth — upstream must not be hit.
	req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for wrong auth, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Errorf("Bad-auth request must not hit upstream, got %d hit(s)", h)
	}

	// 4. Valid auth — must hit upstream exactly once and strip Authorization header.
	req, _ = http.NewRequest("GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for valid auth, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != `{"result":"ok"}` {
		t.Errorf("Unexpected body: %q", string(body))
	}
	if h := atomic.LoadInt64(&upstreamHits); h != 1 {
		t.Errorf("Valid-auth request must hit upstream exactly once, got %d hit(s)", h)
	}
}

// TestProxy_BodyBound verifies that:
//   - bodies at or below the configured limit are accepted and forwarded (upstream hit = 1)
//   - bodies exceeding the limit return 413 before upstream contact (upstream hit = 0)
func TestProxy_BodyBound(t *testing.T) {
	expectedToken := "test-secret-token"
	var upstreamHits int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	const limit int64 = 1024 // 1 KiB — small for test speed

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, limit)
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	// Under limit — upstream must be reached exactly once.
	smallBody := bytes.Repeat([]byte("a"), int(limit))
	req, _ := http.NewRequest("POST", proxyServer.URL+"/", bytes.NewReader(smallBody))
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Under-limit request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Under-limit body must be accepted, got status %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if h := atomic.LoadInt64(&upstreamHits); h != 1 {
		t.Errorf("Under-limit body must hit upstream once, got %d hit(s)", h)
	}

	// Over limit — upstream must receive zero bytes and zero hits.
	atomic.StoreInt64(&upstreamHits, 0)
	bigBody := bytes.Repeat([]byte("b"), int(limit)+1)
	req, _ = http.NewRequest("POST", proxyServer.URL+"/", bytes.NewReader(bigBody))
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Over-limit request failed (network error): %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Over-limit body must be rejected with 413, got status %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Errorf("Over-limit body must not hit upstream, got %d hit(s)", h)
	}
}

// TestProxy_UpstreamResponseHeaderTimeout_Over verifies that a configurable
// response-header-timeout triggers 502 when the upstream is slower than the
// configured budget (millisecond fixture, not minute sleeps).
func TestProxy_UpstreamResponseHeaderTimeout_Over(t *testing.T) {
	expectedToken := "test-secret-token"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond) // slower than proxy timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	// 50ms timeout — well below the 300ms upstream delay.
	handler := NewProxyHandler(targetURL, expectedToken, 50*time.Millisecond, DefaultMaxRequestBodyBytes)
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("Expected 502 Bad Gateway on header timeout, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestProxy_UpstreamResponseHeaderTimeout_Within verifies that a response
// arriving before the configured budget is served correctly (positive case).
func TestProxy_UpstreamResponseHeaderTimeout_Within(t *testing.T) {
	expectedToken := "test-secret-token"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond) // well under 200ms budget
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fast enough"))
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 200*time.Millisecond, DefaultMaxRequestBodyBytes)
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 when response arrives within budget, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestProxy_SlowStream_NotKilledByWriteTimeout verifies that a long chunked
// stream is not cut off by a server write-timeout. NewServer must set both
// WriteTimeout and ReadTimeout to 0.
func TestProxy_SlowStream_NotKilledByWriteTimeout(t *testing.T) {
	expectedToken := "test-secret-token"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("Expected http.Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for i := 0; i < 3; i++ {
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte("chunk\n"))
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)

	// Verify server configuration: WriteTimeout remains 0 for streaming LLM responses,
	// while ReadTimeout is finite (default 30s) to prevent slow-upload DoS.
	server := NewServer(":0", handler)
	if server.WriteTimeout != 0 {
		t.Errorf("WriteTimeout must be 0 for streaming LLM responses, got %v", server.WriteTimeout)
	}
	if server.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout must be %v to prevent slow-upload DoS, got %v", DefaultReadTimeout, server.ReadTimeout)
	}
	if server.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout must be %v, got %v", DefaultReadHeaderTimeout, server.ReadHeaderTimeout)
	}

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Read body failed: %v", err)
	}
	if string(body) != "chunk\nchunk\nchunk\n" {
		t.Errorf("Expected 'chunk\\nchunk\\nchunk\\n', got %q", string(body))
	}
	_ = resp.Body.Close()
}

// TestProxy_StreamCancel verifies that cancelling the client context propagates
// to the upstream via the request context.
func TestProxy_StreamCancel(t *testing.T) {
	expectedToken := "test-secret-token"
	upstreamCanceled := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		select {
		case <-r.Context().Done():
			close(upstreamCanceled)
		case <-time.After(2 * time.Second):
		}
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", proxyServer.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+expectedToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// Cancel client context and close body to trigger upstream cancellation.
	cancel()
	_ = resp.Body.Close()

	select {
	case <-upstreamCanceled:
		// Succeeded: cancellation propagated to upstream.
	case <-time.After(1 * time.Second):
		t.Fatal("Upstream did not detect client cancellation within 1s")
	}
}

// TestProxy_SlowUpload_ReadTimeout_ExpiresBeforeUpstream verifies that when a client
// uploads request body slower than the server's finite ReadTimeout, the server terminates
// the connection before the upstream is ever contacted (upstream hits remain 0).
func TestProxy_SlowUpload_ReadTimeout_ExpiresBeforeUpstream(t *testing.T) {
	expectedToken := "test-secret-token"
	var upstreamHits int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Parse url failed: %v", err)
	}

	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)

	// Start a real server with a tight ReadTimeout (50ms)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	server := NewServer(ln.Addr().String(), handler, 50*time.Millisecond)
	go func() {
		_ = server.Serve(ln)
	}()
	defer server.Close()

	// Connect with raw TCP socket to simulate slowloris / slow-upload
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send valid HTTP headers specifying a 1000-byte body
	header := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Length: 1000\r\n\r\n",
		ln.Addr().String(), expectedToken)
	if _, err := conn.Write([]byte(header)); err != nil {
		t.Fatalf("Write header failed: %v", err)
	}

	// Send initial bytes of body
	_, _ = conn.Write([]byte("initial-chunk"))

	// Sleep past the 50ms ReadTimeout to trigger server-side read deadline expiration
	time.Sleep(100 * time.Millisecond)

	// Try writing more — the server should have aborted reading the body
	_, _ = conn.Write([]byte("-delayed-chunk"))

	// Drain response / read until EOF or timeout
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1024)
	_, _ = conn.Read(buf)

	// Verify upstream hit count remains 0: the slow-upload connection was closed before upstream contact
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Fatalf("Upstream hit count must be 0 on slow-upload read timeout expiry, got %d", h)
	}
}

func TestNewServer_TimeoutConfiguration(t *testing.T) {
	handler := http.NewServeMux()

	// Default read timeout
	s1 := NewServer(":0", handler)
	if s1.ReadTimeout != DefaultReadTimeout {
		t.Errorf("expected default ReadTimeout %v, got %v", DefaultReadTimeout, s1.ReadTimeout)
	}
	if s1.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("expected default ReadHeaderTimeout %v, got %v", DefaultReadHeaderTimeout, s1.ReadHeaderTimeout)
	}
	if s1.WriteTimeout != 0 {
		t.Errorf("expected WriteTimeout 0, got %v", s1.WriteTimeout)
	}

	// Custom read timeout
	customTimeout := 45 * time.Second
	s2 := NewServer(":0", handler, customTimeout)
	if s2.ReadTimeout != customTimeout {
		t.Errorf("expected custom ReadTimeout %v, got %v", customTimeout, s2.ReadTimeout)
	}
}

type mockErrorReader struct {
	data []byte
	err  error
}

func (m *mockErrorReader) Read(p []byte) (n int, err error) {
	if len(m.data) > 0 {
		n = copy(p, m.data)
		m.data = m.data[n:]
		return n, nil
	}
	return 0, m.err
}

func TestProxy_BodyReadError_Mocked(t *testing.T) {
	expectedToken := "test-secret-token"
	var upstreamHits int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)

	// Send request with an io.Reader that fails mid-read
	req := httptest.NewRequest("POST", "/", &mockErrorReader{
		data: []byte("partial data"),
		err:  errors.New("simulated read failure"),
	})
	req.Header.Set("Authorization", "Bearer "+expectedToken)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request on body read error, got %d", resp.StatusCode)
	}
	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Errorf("Upstream hit count must be 0 on body read error, got %d", h)
	}
}

func TestProxy_TruncatedChunk_Rejection(t *testing.T) {
	expectedToken := "test-secret-token"
	var upstreamHits int64

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	targetURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	server := NewServer(ln.Addr().String(), handler, 5*time.Second)
	go func() {
		_ = server.Serve(ln)
	}()
	defer server.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	// Send HTTP request with chunked transfer encoding, but truncate the chunk before finishing
	header := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nTransfer-Encoding: chunked\r\n\r\n",
		ln.Addr().String(), expectedToken)
	if _, err := conn.Write([]byte(header)); err != nil {
		t.Fatalf("Write header failed: %v", err)
	}

	// Chunk size 10 (0xa), but only send 4 bytes and close connection abruptly
	_, _ = conn.Write([]byte("a\r\n1234"))
	_ = conn.Close() // abrupt termination = truncated chunk

	// Give server a moment to finish processing
	time.Sleep(50 * time.Millisecond)

	if h := atomic.LoadInt64(&upstreamHits); h != 0 {
		t.Fatalf("Upstream hit count must be 0 on truncated chunk body, got %d", h)
	}
}
