package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/client"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKratosClient implements a mock for testing
type mockKratosClient struct {
	getFirstIdentityIDFunc func(ctx context.Context) (string, error)
}

func (m *mockKratosClient) Whoami(ctx context.Context, cookie string) (*client.Identity, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKratosClient) GetFirstIdentityID(ctx context.Context) (string, error) {
	if m.getFirstIdentityIDFunc != nil {
		return m.getFirstIdentityIDFunc(ctx)
	}
	return "", errors.New("not configured")
}

func TestNewInternalHandler(t *testing.T) {
	t.Run("creates handler with kratos client", func(t *testing.T) {
		kratosClient := client.NewKratosClientWithAdmin("http://kratos:4433", "http://kratos:4434", 5*time.Second)
		handler := NewInternalHandler(kratosClient)

		assert.NotNil(t, handler)
	})
}

func TestInternalHandler_HandleSystemUser(t *testing.T) {
	t.Run("success - returns user ID", func(t *testing.T) {
		kratosClient := client.NewKratosClientWithAdmin("http://unused", "http://unused", 5*time.Second)
		handler := NewInternalHandler(kratosClient)

		// Create a mock server for Kratos Admin API
		mockKratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := []map[string]any{
				{
					"id":        "user-123-uuid",
					"schema_id": "default",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer mockKratos.Close()

		// Override client with mock server URL
		kratosClient = client.NewKratosClientWithAdmin("http://unused", mockKratos.URL, 5*time.Second)
		handler = NewInternalHandler(kratosClient)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.HandleSystemUser(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

		var response SystemUserResponse
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "user-123-uuid", response.UserID)
	})

	t.Run("error - kratos returns error", func(t *testing.T) {
		// Mock Kratos server that returns 500
		mockKratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer mockKratos.Close()

		kratosClient := client.NewKratosClientWithAdmin("http://unused", mockKratos.URL, 5*time.Second)
		handler := NewInternalHandler(kratosClient)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.HandleSystemUser(c)

		require.NoError(t, err) // Echo handles the error via JSON response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var response map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to fetch system user", response["error"])
	})

	t.Run("error - no identities found", func(t *testing.T) {
		// Mock Kratos server that returns empty list
		mockKratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer mockKratos.Close()

		kratosClient := client.NewKratosClientWithAdmin("http://unused", mockKratos.URL, 5*time.Second)
		handler := NewInternalHandler(kratosClient)

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/internal/system-user", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler.HandleSystemUser(c)

		require.NoError(t, err) // Echo handles the error via JSON response
		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var response map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Failed to fetch system user", response["error"])
	})
}
