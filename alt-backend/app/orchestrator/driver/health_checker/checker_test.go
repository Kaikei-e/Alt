package health_checker

import (
	"alt/domain"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckHealth(t *testing.T) {
	t.Run("returns healthy for 200 OK", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
		}))
		defer srv.Close()

		checker := NewChecker([]ServiceEndpoint{
			{Name: "test-service", Endpoint: srv.URL + "/health"},
		})

		results, err := checker.CheckHealth(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, "test-service", results[0].ServiceName)
		assert.Equal(t, domain.ServiceHealthy, results[0].Status)
		assert.Greater(t, results[0].LatencyMs, int64(-1))
		assert.Empty(t, results[0].ErrorMessage)
	})

	t.Run("returns unhealthy for 503", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		checker := NewChecker([]ServiceEndpoint{
			{Name: "down-service", Endpoint: srv.URL + "/health"},
		})

		results, err := checker.CheckHealth(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, domain.ServiceUnhealthy, results[0].Status)
		assert.Contains(t, results[0].ErrorMessage, "503")
	})

	t.Run("returns unhealthy for connection refused", func(t *testing.T) {
		checker := NewChecker([]ServiceEndpoint{
			{Name: "unreachable", Endpoint: "http://127.0.0.1:1/health"},
		})

		results, err := checker.CheckHealth(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 1)

		assert.Equal(t, domain.ServiceUnhealthy, results[0].Status)
		assert.NotEmpty(t, results[0].ErrorMessage)
	})

	t.Run("checks multiple services concurrently", func(t *testing.T) {
		healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer healthy.Close()

		unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer unhealthy.Close()

		checker := NewChecker([]ServiceEndpoint{
			{Name: "svc-a", Endpoint: healthy.URL + "/health"},
			{Name: "svc-b", Endpoint: unhealthy.URL + "/health"},
		})

		results, err := checker.CheckHealth(context.Background())
		require.NoError(t, err)
		require.Len(t, results, 2)

		// Results are ordered by input order
		assert.Equal(t, domain.ServiceHealthy, results[0].Status)
		assert.Equal(t, domain.ServiceUnhealthy, results[1].Status)
	})
}
