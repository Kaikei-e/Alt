package datahub

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
	"alt/dataplane/driver/kratos_client"
	"alt/di"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
)

func mounted(t *testing.T, mux *http.ServeMux, path string) bool {
	t.Helper()
	_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, path, nil))
	return pattern != ""
}

// components returns the smallest container SetupConnectHandlers accepts.
//
// The two absorbed /v1/internal capabilities are required rather than
// optional, so unlike the rest of the container they cannot be left nil here:
// a nil there is a composition-root bug, and the setup panics on it by design
// (ADR-000954 D6, CLAUDE.md rule 8). They are wired to zero-valued instances
// because these tests assert routing, not behaviour.
func components() *di.DataHubComponents {
	return &di.DataHubComponents{
		KratosClient:               kratos_client.NewKratosClient("", ""),
		FetchRecentArticlesUsecase: fetch_recent_articles_usecase.NewFetchRecentArticlesUsecase(nil),
	}
}

// data-hub's mux carries the service-to-service surfaces and nothing else. The
// admin services it used to share a listener with are guarded by a loopback
// bind rather than a client certificate, and the user services are guarded by
// a JWT; mixing any of them onto one socket makes "who may call this" a
// question with three different answers.
//
// Two service-to-service namespaces are mounted, not one. ADR-000954 D7 keeps
// services.backend.v1 alive until Wave 2-B has moved all seven peers onto
// alt.datahub.v1, and a peer whose PR has not landed yet must keep working.
// Wave 2-C inverts the legacy expectation here.
func TestSetupConnectHandlers_ServesBothServiceToServiceNamespaces(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()

	SetupConnectHandlers(mux, components(), &config.Config{}, logger)

	tests := []struct {
		name        string
		path        string
		wantMounted bool
	}{
		{name: "datahub service", path: "/alt.datahub.v1.DataHubService/CreateArticle", wantMounted: true},
		{name: "datahub service absorbs the internal REST reads", path: "/alt.datahub.v1.DataHubService/ListRecentArticles", wantMounted: true},
		{name: "legacy backend internal service stays mounted until Wave 2-C", path: "/services.backend.v1.BackendInternalService/CreateArticle", wantMounted: true},
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

// An unwired absorbed capability must stop the process, not answer requests.
// Returning Unimplemented instead would be indistinguishable from a procedure
// that was deliberately retired — the ADR-000928 failure mode CLAUDE.md rule 8
// names directly.
func TestSetupConnectHandlers_RefusesToMountWithoutTheAbsorbedRESTCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		container *di.DataHubComponents
	}{
		{
			name: "no kratos client",
			container: &di.DataHubComponents{
				FetchRecentArticlesUsecase: fetch_recent_articles_usecase.NewFetchRecentArticlesUsecase(nil),
			},
		},
		{
			name: "no recent articles usecase",
			container: &di.DataHubComponents{
				KratosClient: kratos_client.NewKratosClient("", ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("SetupConnectHandlers returned instead of panicking on an unwired dependency")
				}
			}()
			SetupConnectHandlers(http.NewServeMux(), tt.container, &config.Config{},
				slog.New(slog.NewTextHandler(io.Discard, nil)))
		})
	}
}

// CreateServer is the only constructor this package offers, and it adds only
// the health endpoint on top of SetupConnectHandlers.
func TestCreateServer_AddsHealthAndNothingElse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := CreateServer(components(), &config.Config{}, logger)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", rec.Code)
	}
}
