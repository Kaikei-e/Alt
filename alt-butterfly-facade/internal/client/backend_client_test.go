package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient creates a client with HTTP/1.1 transport for testing
func newTestClient(baseURL string) *BackendClient {
	return NewBackendClientWithTransport(baseURL, 30*time.Second, 5*time.Minute, http.DefaultTransport)
}

func TestNewBackendClient(t *testing.T) {
	client := NewBackendClient("http://localhost:9101", 30*time.Second, 5*time.Minute)

	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:9101", client.baseURL)
}

func TestBackendClient_HttpClientTimeout_IsZero(t *testing.T) {
	// httpClient.Timeout must be 0 (disabled) so that Connect-Timeout-Ms
	// header-based context deadlines are the sole timeout mechanism.
	client := NewBackendClientWithTransport("http://localhost:9101", 30*time.Second, 5*time.Minute, http.DefaultTransport)

	assert.Equal(t, time.Duration(0), client.httpClient.Timeout,
		"httpClient.Timeout should be 0; timeout is controlled via context deadline from Connect-Timeout-Ms")
}

func TestBackendClient_DefaultTimeout(t *testing.T) {
	client := NewBackendClientWithTransport("http://localhost:9101", 30*time.Second, 5*time.Minute, http.DefaultTransport)

	assert.Equal(t, 30*time.Second, client.DefaultTimeout())
}

func TestBackendClient_ForwardRequest_Success(t *testing.T) {
	// Create a mock backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers are forwarded
		assert.Equal(t, "test-token", r.Header.Get("X-Alt-Backend-Token"))
		assert.Equal(t, "application/proto", r.Header.Get("Content-Type"))

		// Read body
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "request-body", string(body))

		// Send response
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response-body"))
	}))
	defer backend.Close()

	client := newTestClient(backend.URL)

	// Create request
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		backend.URL+"/alt.feeds.v2.FeedService/GetFeedStats",
		strings.NewReader("request-body"),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/proto")

	// Forward request
	resp, err := client.ForwardRequest(req, "test-token")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, "response-body", string(body))
}

func TestBackendClient_ForwardRequest_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer backend.Close()

	client := newTestClient(backend.URL)

	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		backend.URL+"/test",
		nil,
	)

	resp, err := client.ForwardRequest(req, "token")

	require.NoError(t, err) // No error, just status code
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	resp.Body.Close()
}

func TestBackendClient_ForwardRequest_Timeout(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// httpClient.Timeout is 0 (disabled), so timeout is controlled via context
	client := NewBackendClientWithTransport(backend.URL, 10*time.Millisecond, 10*time.Millisecond, http.DefaultTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, backend.URL+"/test", nil)

	_, err := client.ForwardRequest(req, "token")

	assert.Error(t, err)
}

func TestBackendClient_BuildBackendURL(t *testing.T) {
	client := newTestClient("http://localhost:9101")

	tests := []struct {
		path     string
		expected string
	}{
		{"/alt.feeds.v2.FeedService/GetFeedStats", "http://localhost:9101/alt.feeds.v2.FeedService/GetFeedStats"},
		{"alt.feeds.v2.FeedService/GetFeedStats", "http://localhost:9101/alt.feeds.v2.FeedService/GetFeedStats"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := client.BuildBackendURL(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBackendClient_ForwardStreamingRequest(t *testing.T) {
	// Create a mock streaming backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-token", r.Header.Get("X-Alt-Backend-Token"))

		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Send streaming chunks
		for i := 0; i < 3; i++ {
			w.Write([]byte("chunk"))
			flusher.Flush()
		}
	}))
	defer backend.Close()

	client := newTestClient(backend.URL)

	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		backend.URL+"/alt.feeds.v2.FeedService/StreamFeedStats",
		nil,
	)

	resp, err := client.ForwardStreamingRequest(req, "test-token")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestCopyHeaders_ExcludesAcceptEncoding(t *testing.T) {
	// Accept-Encoding should NOT be forwarded to backend.
	// Go's http2.Transport does not auto-decompress when Accept-Encoding is set manually,
	// which causes gzip-compressed responses to be forwarded without decompression.
	src := http.Header{}
	src.Set("Content-Type", "application/proto")
	src.Set("Accept", "application/proto")
	src.Set("Accept-Encoding", "gzip, deflate, br") // Should be excluded
	src.Set("Connect-Protocol-Version", "1")

	dst := http.Header{}
	copyHeaders(src, dst)

	assert.Equal(t, "application/proto", dst.Get("Content-Type"))
	assert.Equal(t, "application/proto", dst.Get("Accept"))
	assert.Equal(t, "1", dst.Get("Connect-Protocol-Version"))
	assert.Empty(t, dst.Get("Accept-Encoding"), "Accept-Encoding should not be forwarded")
}

func TestCopyHeaders_ForwardsRequiredHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "application/connect+proto")
	src.Set("Accept", "application/connect+proto")
	src.Set("Connect-Protocol-Version", "1")
	src.Set("Connect-Timeout-Ms", "30000")
	src.Set("Grpc-Timeout", "30S")

	dst := http.Header{}
	copyHeaders(src, dst)

	assert.Equal(t, "application/connect+proto", dst.Get("Content-Type"))
	assert.Equal(t, "application/connect+proto", dst.Get("Accept"))
	assert.Equal(t, "1", dst.Get("Connect-Protocol-Version"))
	assert.Equal(t, "30000", dst.Get("Connect-Timeout-Ms"))
	assert.Equal(t, "30S", dst.Get("Grpc-Timeout"))
}
