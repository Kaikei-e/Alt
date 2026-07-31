package datahub

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
	"alt/di"
)

func mounted(t *testing.T, mux *http.ServeMux, path string) bool {
	t.Helper()
	_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, path, nil))
	return pattern != ""
}

// data-hub's mux carries BackendInternalService and nothing else. The admin
// services it used to share a listener with are guarded by a loopback bind
// rather than a client certificate, and the user services are guarded by a
// JWT; mixing any of them onto one socket makes "who may call this" a question
// with three different answers.
func TestSetupConnectHandlers_ServesOnlyBackendInternalService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()

	SetupConnectHandlers(mux, &di.DataHubComponents{}, &config.Config{}, logger)

	tests := []struct {
		name        string
		path        string
		wantMounted bool
	}{
		{name: "backend internal service", path: "/services.backend.v1.BackendInternalService/CreateArticle", wantMounted: true},
		{name: "admin surface belongs to the backend operator listener", path: "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth"},
		{name: "user feed service", path: "/alt.feeds.v2.FeedService/GetFeedStats"},
		{name: "user knowledge home service", path: "/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mounted(t, mux, tt.path); got != tt.wantMounted {
				t.Errorf("%s mounted = %v, want %v", tt.path, got, tt.wantMounted)
			}
		})
	}
}

// CreateServer is the only constructor this package offers, and it adds only
// the health endpoint on top of SetupConnectHandlers.
func TestCreateServer_AddsHealthAndNothingElse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := CreateServer(&di.DataHubComponents{}, &config.Config{}, logger)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", rec.Code)
	}
}
