package dashboard_gateway

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDashboardQueryURL(t *testing.T) {
	t.Run("build metrics URL with all params", func(t *testing.T) {
		u, err := buildDashboardQueryURL("http://recap-worker:9005", "/v1/dashboard/metrics", "memory", 3600, 50)
		require.NoError(t, err)
		assert.Contains(t, u, "http://recap-worker:9005/v1/dashboard/metrics?")
		assert.Contains(t, u, "type=memory")
		assert.Contains(t, u, "window=3600")
		assert.Contains(t, u, "limit=50")
	})

	t.Run("build overview URL with default params", func(t *testing.T) {
		u, err := buildDashboardQueryURL("http://recap-worker:9005", "/v1/dashboard/overview", "", 0, 0)
		require.NoError(t, err)
		assert.Equal(t, "http://recap-worker:9005/v1/dashboard/overview", u)
	})
}

func TestCheckDashboardStatus(t *testing.T) {
	t.Run("status OK returns nil", func(t *testing.T) {
		err := checkDashboardStatus(http.StatusOK, []byte("ok"))
		assert.NoError(t, err)
	})

	t.Run("non-200 returns error with status and body", func(t *testing.T) {
		err := checkDashboardStatus(http.StatusInternalServerError, []byte("server down"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "server down")
	})
}
