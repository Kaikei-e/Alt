package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllowConnectPath_ReadingServiceRecordReadingEvents(t *testing.T) {
	for _, path := range []string{
		"/alt.reading.v1.ReadingService/RecordReadingEvents",
		"/alt.reading.v1.ReadingService/QueueArticle",
		"/alt.reading.v1.ReadingService/UnqueueArticle",
		"/alt.reading.v1.ReadingService/GetReadTodayQueue",
	} {
		assert.True(t, allowConnectPath(path), "path %q", path)
	}
}

func TestServer_ForwardsReadingServiceRecordReadingEvents(t *testing.T) {
	var seenPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer backend.Close()

	cfg := Config{
		BackendURL:       backend.URL,
		TokenVerifyKey:   testPublicKey,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}
	h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	const path = "/alt.reading.v1.ReadingService/RecordReadingEvents"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, testPrivateKey))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, path, seenPath)
}

func TestAllowConnectPath_FeedInflowAndDailyCapRPCs(t *testing.T) {
	for _, path := range []string{
		"/alt.feeds.v2.FeedService/SetSubscriptionDailyCap",
		"/alt.feeds.v2.FeedService/GetFeedInflowStats",
	} {
		assert.True(t, allowConnectPath(path), "path %q", path)
	}
}

func TestServer_ForwardsFeedInflowAndDailyCapRPCs(t *testing.T) {
	for _, tc := range []struct {
		path string
		body string
	}{
		{
			path: "/alt.feeds.v2.FeedService/SetSubscriptionDailyCap",
			body: `{"feedLinkId":"11111111-1111-4111-8111-111111111111","dailyCap":3}`,
		},
		{
			path: "/alt.feeds.v2.FeedService/GetFeedInflowStats",
			body: `{}`,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var seenPath string
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer backend.Close()

			cfg := Config{
				BackendURL:       backend.URL,
				TokenVerifyKey:   testPublicKey,
				Issuer:           "auth-hub",
				Audience:         "alt-backend",
				RequestTimeout:   30 * time.Second,
				StreamingTimeout: 5 * time.Minute,
			}
			h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createValidToken(t, testPrivateKey))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.path, seenPath)
		})
	}
}
