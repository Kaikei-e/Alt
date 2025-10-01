package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKratosClient(t *testing.T) {
	t.Run("creates client with valid URL", func(t *testing.T) {
		client := NewKratosClient("http://kratos:4433", 5*time.Second)

		assert.NotNil(t, client)
	})
}

func TestKratosClient_Whoami(t *testing.T) {
	t.Run("successful session validation returns identity", func(t *testing.T) {
		// Mock Kratos server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/sessions/whoami", r.URL.Path)
			assert.Equal(t, "GET", r.Method)
			assert.Contains(t, r.Header.Get("Cookie"), "ory_kratos_session=valid-session")

			response := map[string]any{
				"id":     "session-123",
				"active": true,
				"identity": map[string]any{
					"id": "user-456",
					"traits": map[string]any{
						"email": "user@example.com",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=valid-session")

		require.NoError(t, err)
		assert.NotNil(t, identity)
		assert.Equal(t, "user-456", identity.ID)
		assert.Equal(t, "user@example.com", identity.Email)
	})

	t.Run("inactive session returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":     "session-123",
				"active": false,
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=expired-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "session is not active")
	})

	t.Run("401 unauthorized returns authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "unauthorized",
			})
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=invalid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "authentication failed")
	})

	t.Run("missing cookie returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("should not reach Kratos with empty cookie")
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "cookie cannot be empty")
	})

	t.Run("HTTP client timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Client with very short timeout
		client := NewKratosClient(server.URL, 50*time.Millisecond)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=valid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})

	t.Run("invalid JSON response returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"invalid json`))
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=valid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "failed to parse response")
	})

	t.Run("500 internal server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "internal server error",
			})
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=valid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "kratos returned status 500")
	})

	t.Run("missing identity in response returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":     "session-123",
				"active": true,
				// Missing identity field
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)
		identity, err := client.Whoami(context.Background(), "ory_kratos_session=valid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "missing identity")
	})
}

func TestKratosClient_ContextCancellation(t *testing.T) {
	t.Run("cancelled context returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewKratosClient(server.URL, 5*time.Second)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		identity, err := client.Whoami(ctx, "ory_kratos_session=valid-session")

		assert.Error(t, err)
		assert.Nil(t, identity)
		assert.Contains(t, err.Error(), "context canceled")
	})
}
