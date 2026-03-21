package rag_augur

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaEmbedder_Encode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embed", r.URL.Path)

		var req embedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test-model", req.Model)
		assert.Equal(t, []string{"hello world"}, req.Input)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(embedResponse{
			Embeddings: [][]float32{{0.1, 0.2, 0.3}},
		})
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 10)

	vecs, err := embedder.Encode(context.Background(), []string{"hello world"})
	require.NoError(t, err)
	assert.Len(t, vecs, 1)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, vecs[0])
}

func TestOllamaEmbedder_Encode_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 10)

	_, err := embedder.Encode(context.Background(), []string{"hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama returned status: 503")
}

func TestOllamaEmbedder_Encode_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Exceed timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Very short timeout to trigger timeout error
	client := &http.Client{Timeout: 50 * time.Millisecond}
	embedder := NewOllamaEmbedder(server.URL, "test-model", 0, client)

	_, err := embedder.Encode(context.Background(), []string{"hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ollama (client_timeout)")
}

func TestOllamaEmbedder_Encode_ContextDeadlineExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 10)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := embedder.Encode(ctx, []string{"hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ollama (context_deadline_exceeded)")
}

func TestOllamaEmbedder_Encode_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := embedder.Encode(ctx, []string{"hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ollama (context_canceled)")
}

func TestOllamaEmbedder_Encode_DecodeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 10)

	_, err := embedder.Encode(context.Background(), []string{"hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestOllamaEmbedder_Version(t *testing.T) {
	embedder := NewOllamaEmbedder("http://localhost", "my-model", 10)
	assert.Equal(t, "my-model", embedder.Version())
}

func TestClassifyTransportError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "context_deadline_exceeded",
			err:      context.DeadlineExceeded,
			expected: "context_deadline_exceeded",
		},
		{
			name:     "wrapped_context_deadline_exceeded",
			err:      fmt.Errorf("request failed: %w", context.DeadlineExceeded),
			expected: "context_deadline_exceeded",
		},
		{
			name:     "context_canceled",
			err:      context.Canceled,
			expected: "context_canceled",
		},
		{
			name:     "wrapped_context_canceled",
			err:      fmt.Errorf("request failed: %w", context.Canceled),
			expected: "context_canceled",
		},
		{
			name:     "client_timeout",
			err:      fmt.Errorf("context deadline exceeded (Client.Timeout exceeded while awaiting headers): %w", context.DeadlineExceeded),
			expected: "client_timeout",
		},
		{
			name: "connection_failed_dial",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Addr: &net.TCPAddr{
					IP:   net.ParseIP("127.0.0.1"),
					Port: 11436,
				},
				Err: errors.New("connection refused"),
			},
			expected: "connection_failed",
		},
		{
			name:     "other_transport_error",
			err:      errors.New("some unknown transport issue"),
			expected: "transport_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifyTransportError(tt.err))
		})
	}
}
