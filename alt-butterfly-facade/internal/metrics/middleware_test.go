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

func TestHandler_WriteDoesNotHTMLEscapeJSONBodies(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	jsonBody := []byte(`{"q":"<script>alert(1)</script>"}`)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonBody)
	})

	rec := httptest.NewRecorder()
	m.Handler(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetAllFeeds", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, string(jsonBody), rec.Body.String())
	assert.Equal(t, "application/json", rec.Result().Header.Get("Content-Type"))
}

func TestHandler_WritePreservesHandlerContentType(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	const wantType = "application/connect+proto"
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", wantType)
		_, _ = w.Write([]byte{0x00, 0x01, 0x02})
	})

	rec := httptest.NewRecorder()
	m.Handler(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetAllFeeds", nil))

	assert.Equal(t, wantType, rec.Result().Header.Get("Content-Type"))
	assert.Equal(t, []byte{0x00, 0x01, 0x02}, rec.Body.Bytes())
}

func TestHandler_WriteDoesNotInventContentType(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	body := []byte(`{"q":"<script>alert(1)</script>"}`)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/GetAllFeeds", nil)

	plain := httptest.NewRecorder()
	inner.ServeHTTP(plain, req)

	wrapped := httptest.NewRecorder()
	m.Handler(inner).ServeHTTP(wrapped, req)

	assert.Equal(t, plain.Result().Header.Get("Content-Type"), wrapped.Result().Header.Get("Content-Type"))
	assert.Equal(t, string(body), wrapped.Body.String())
}
