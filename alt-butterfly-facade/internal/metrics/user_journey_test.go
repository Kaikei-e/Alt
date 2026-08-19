package metrics

import (
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyForPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/alt.feeds.v2.FeedService/GetAllFeeds", want: JourneyFeeds},
		{path: "/alt.feeds.v2.FeedService/MarkAsRead", want: JourneyFeeds},
		{path: "/v1/feeds/read", want: JourneyFeeds},
		{path: "/v1/feeds/stats/trends", want: JourneyFeeds},
		{path: "/v1/feeds/abc/tags", want: JourneyFeeds},
		{path: "/alt.search.v2.SearchService/Search", want: JourneySearch},
		{path: "/alt.search.v2.GlobalSearchService/Search", want: JourneySearch},
		{path: "/alt.articles.v2.ArticleService/GetArticle", want: ""},
		{path: "/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome", want: ""},
		{path: "/health", want: ""},
		{path: "/metrics", want: ""},
		{path: "/ory/sessions/whoami", want: JourneyLogin},
		{path: "/sessions/whoami", want: JourneyLogin},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, JourneyForPath(tt.path))
		})
	}
}

func TestRecord_FeedsOKDoesNotInflateLogin(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusOK)

	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusError)))
}

func TestRecord_Feeds5xxIsFeedsErrorNotLoginOK(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusBadGateway)
	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusGatewayTimeout)
	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusServiceUnavailable)

	assert.Equal(t, 3.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusError)))
}

func TestRecord_SearchErrorDoesNotCountAsFeeds(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.search.v2.SearchService/Search", http.StatusInternalServerError)

	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneySearch, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
}

func TestRecord_UnauthorizedIsLoginAndJourneyError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusUnauthorized)
	m.Record("/alt.search.v2.SearchService/Search", http.StatusUnauthorized)

	assert.Equal(t, 2.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusError)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusError)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneySearch, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusOK)))
}

func TestRecord_UnclassifiedSuccessIsNotLogin(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.articles.v2.ArticleService/GetArticle", http.StatusOK)
	m.Record("/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome", http.StatusOK)

	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneySearch, StatusOK)))
}

func TestRecord_SessionPathSuccessIsLoginOKOnly(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/ory/sessions/whoami", http.StatusOK)
	m.Record("/sessions/whoami", http.StatusOK)

	assert.Equal(t, 2.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusOK)))
}

func TestRecord_SessionPathFailureIsLoginError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/ory/sessions/whoami", http.StatusUnauthorized)
	m.Record("/sessions/whoami", http.StatusBadGateway)

	assert.Equal(t, 2.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusError)))
}

func TestNew_DoesNotPrecreateZeroRequestSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = New(reg)

	families, err := reg.Gather()
	require.NoError(t, err)

	var foundInstrumented bool
	for _, fam := range families {
		switch fam.GetName() {
		case instrumentedName:
			foundInstrumented = true
			require.Len(t, fam.GetMetric(), 1)
			assert.InDelta(t, 1.0, fam.GetMetric()[0].GetGauge().GetValue(), 0.001)
		case metricName:
			assert.Empty(t, fam.GetMetric(), "pre-created zeros must not fake journey coverage")
		}
	}
	assert.True(t, foundInstrumented, "instrumented gauge must exist so scrape absence can page")
}

func TestClientErrorIsNotSLOError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusBadRequest)
	m.Record("/alt.feeds.v2.FeedService/GetAllFeeds", http.StatusForbidden)

	assert.Equal(t, 2.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusOK)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyFeeds, StatusError)))
}

func TestTooManyRequestsIsJourneyError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.Record("/alt.search.v2.SearchService/Search", http.StatusTooManyRequests)

	assert.Equal(t, 1.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneySearch, StatusError)))
	assert.Equal(t, 0.0, testutil.ToFloat64(m.requests.WithLabelValues(JourneyLogin, StatusOK)))
}
