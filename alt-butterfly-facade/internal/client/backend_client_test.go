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

	// Very short timeout
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
