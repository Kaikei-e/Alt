package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestHandler_RecordsStatusFromDownstream(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream", http.StatusBadGateway)
	})
	wrapped := m.Handler(inner)

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetAllFeeds", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusError)))
}

func TestHandler_NilMetricsIsPassthrough(t *testing.T) {
	var m *Metrics
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	m.Handler(inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.True(t, called)
}
