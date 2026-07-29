package v2

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/config"
	"alt/di"
)

// internalServicePaths are the Connect-RPC surfaces that carry no user-JWT
// interceptor. Mounting them on the browser-facing listener made them
// reachable by any self-registered user through the product's own
// /api/v2 proxy, which forwards an arbitrary service/method path with the
// caller's valid token attached.
var internalServicePaths = []string{
	"/services.backend.v1.BackendInternalService/CreateArticle",
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

	for _, path := range internalServicePaths {
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

func TestSetupInternalConnectHandlers_ServesOnlyInternalAndAdminServices(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupInternalConnectHandlers(mux, container, cfg, logger)

	for _, path := range internalServicePaths {
		if !mounted(t, mux, path) {
			t.Errorf("%s must be reachable on the internal mux", path)
		}
	}
	for _, path := range publicServicePaths {
		if mounted(t, mux, path) {
			t.Errorf("%s must not be duplicated onto the internal mux", path)
		}
	}
}

// The mTLS listener verifies the client certificate against alt-ca before any
// handler runs, so it is the one place both surfaces may coexist.
func TestCreateMTLSConnectServer_ServesBothSurfaces(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupConnectHandlers(mux, container, cfg, logger)
	SetupInternalConnectHandlers(mux, container, cfg, logger)

	for _, path := range append(append([]string{}, internalServicePaths...), publicServicePaths...) {
		if !mounted(t, mux, path) {
			t.Errorf("%s must be reachable on the mTLS mux", path)
		}
	}
}
