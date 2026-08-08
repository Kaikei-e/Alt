package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHTTPServer_ServesNoMetricsOnTheAPIListener keeps the notification-outbox
// gauges off the service API listener. That listener's access control is "who
// can open a socket", so every route added to it is a new unauthenticated
// surface — the sibling services' topology suites assert a 404 on /metrics for
// exactly that reason. The gauges belong on the dedicated metrics listener
// (:9201, /metrics/prometheus) instead.
func TestHTTPServer_ServesNoMetricsOnTheAPIListener(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	for _, path := range []string{"/metrics", "/metrics/prometheus"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()

		srv.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code,
			"the API listener must not serve %s; metrics live on the dedicated listener", path)
	}
}
