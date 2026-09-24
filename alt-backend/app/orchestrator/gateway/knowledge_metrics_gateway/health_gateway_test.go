package knowledge_metrics_gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/domain"
	"alt/orchestrator/driver/health_checker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceHealthGateway_CheckHealth(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer healthySrv.Close()

	unhealthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthySrv.Close()

	checker := health_checker.NewChecker([]health_checker.ServiceEndpoint{
		{Name: "healthy-svc", Endpoint: healthySrv.URL + "/health"},
		{Name: "unhealthy-svc", Endpoint: unhealthySrv.URL + "/health"},
		{Name: "unknown-svc", Endpoint: "://bad-url"},
	})
	gw := NewServiceHealthGateway(checker)

	statuses, err := gw.CheckHealth(context.Background())
	require.NoError(t, err)
	require.Len(t, statuses, 3)

	// Healthy mapping
	assert.Equal(t, "healthy-svc", statuses[0].ServiceName)
	assert.Equal(t, domain.ServiceHealthy, statuses[0].Status)
	assert.Equal(t, healthySrv.URL+"/health", statuses[0].Endpoint)

	// Unhealthy mapping
	assert.Equal(t, "unhealthy-svc", statuses[1].ServiceName)
	assert.Equal(t, domain.ServiceUnhealthy, statuses[1].Status)
	assert.NotEmpty(t, statuses[1].ErrorMessage)

	// Unmapped -> unknown mapping
	assert.Equal(t, "unknown-svc", statuses[2].ServiceName)
	assert.Equal(t, domain.ServiceUnknown, statuses[2].Status)
	assert.NotEmpty(t, statuses[2].ErrorMessage)
}
