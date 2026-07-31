package v2

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
	"alt/di"
)

// dataHubServicePaths are the service-to-service surfaces. They carry no
// user-JWT interceptor and now live on their own binary behind mTLS.
var dataHubServicePaths = []string{
	"/alt.datahub.v1.DataHubService/CreateArticle",
}

// operatorServicePaths are the admin surfaces. They carry no user-JWT
// interceptor either, but their control is the backend's loopback bind, not a
// client certificate — so they must not share a mux with the data-hub ones.
var operatorServicePaths = []string{
	"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
}

// publicServicePaths are the user-facing surfaces guarded by the JWT auth
// interceptor.
var publicServicePaths = []string{
	"/alt.feeds.v2.FeedService/GetFeedStats",
	"/alt.articles.v2.ArticleService/GetArticle",
	"/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
}

func testDeps() (*di.ApplicationComponents, *config.Config, *slog.Logger) {
	return &di.ApplicationComponents{}, &config.Config{}, slog.New(slog.NewTextHandler(io_Discard{}, nil))
}

type io_Discard struct{}

func (io_Discard) Write(p []byte) (int, error) { return len(p), nil }

func mounted(t *testing.T, mux *http.ServeMux, path string) bool {
	t.Helper()
	_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, path, nil))
	return pattern != ""
}

func TestSetupConnectHandlers_ExcludesInternalAndAdminServices(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)

	for _, path := range append(append([]string{}, dataHubServicePaths...), operatorServicePaths...) {
		if mounted(t, mux, path) {
			t.Errorf("%s must not be reachable on the browser-facing mux", path)
		}
	}
	for _, path := range publicServicePaths {
		if !mounted(t, mux, path) {
			t.Errorf("%s must stay reachable on the browser-facing mux", path)
		}
	}
}

// The pre-split internal mux carried the admin surfaces and the data-plane
// service together, which meant one listener answered to two
// different access controls. They are separate muxes on separate binaries now,
// and neither may re-acquire the other's services.
func TestSetupOperatorConnectHandlers_ServesOnlyTheAdminSurfaces(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupOperatorConnectHandlers(mux, container, cfg, logger)

	for _, path := range operatorServicePaths {
		if !mounted(t, mux, path) {
			t.Errorf("%s must be reachable on the operator mux", path)
		}
	}
	for _, path := range append(append([]string{}, dataHubServicePaths...), publicServicePaths...) {
		if mounted(t, mux, path) {
			t.Errorf("%s must not be mounted on the operator mux", path)
		}
	}
}

// The mixed-surface mTLS server was the direct motivation for splitting the
// binary: one listener served the user API, the admin API and the
// service-to-service API at once. Neither mux this package builds may carry
// the other's services, nor the data-hub ones — whose handler lives in
// alt/connect/v2/datahub and is not even linked into this package.
func TestConnectServers_NeverShareSurfaces(t *testing.T) {
	container, cfg, logger := testDeps()

	userMux := http.NewServeMux()
	SetupConnectHandlers(userMux, container, cfg, logger)
	operatorMux := http.NewServeMux()
	SetupOperatorConnectHandlers(operatorMux, container, cfg, logger)

	surfaces := []struct {
		name  string
		mux   *http.ServeMux
		owns  []string
		alien []string
	}{
		{name: "user", mux: userMux, owns: publicServicePaths,
			alien: append(append([]string{}, operatorServicePaths...), dataHubServicePaths...)},
		{name: "operator", mux: operatorMux, owns: operatorServicePaths,
			alien: append(append([]string{}, publicServicePaths...), dataHubServicePaths...)},
	}

	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			for _, p := range s.owns {
				if !mounted(t, s.mux, p) {
					t.Errorf("%s mux lost %s", s.name, p)
				}
			}
			for _, p := range s.alien {
				if mounted(t, s.mux, p) {
					t.Errorf("%s mux must not serve %s", s.name, p)
				}
			}
		})
	}
}
