package v2

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alt/config"
	"alt/di"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/shared/gateway/datahub_gateway"
)

// dataHubServicePaths are the service-to-service surfaces. They carry no
// user-JWT interceptor and now live on their own binary behind mTLS.
var dataHubServicePaths = []string{
	"/services.datahub.v1.DataHubService/CreateArticle",
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
	"/alt.push.v1.PushService/GetPushConfig",
}

// testDeps returns the smallest container SetupConnectHandlers accepts.
//
// Infra must be non-nil: the article handler takes its OG image lookup from
// there since ADR-000954 Wave 3 moved article_heads to alt-data-hub, and these
// tests assert routing rather than behaviour, so a zero-valued module is
// enough.
// PushSubscriptionGateway and VAPID_PUBLIC_KEY are the exception to that: both
// are refused at construction, because a nil port or an empty key produces a
// push surface that answers on the wire and fails only in the browser. They are
// wired to a client pointed at an address nothing resolves, which is enough for
// routing assertions and would fail loudly if one of these tests ever made a
// call.
func testDeps() (*di.ApplicationComponents, *config.Config, *slog.Logger) {
	pushGateway := datahub_gateway.NewPushSubscriptionGateway(
		datahubv1connect.NewDataHubServiceClient(http.DefaultClient, "http://datahub.invalid"))

	cfg := &config.Config{}
	cfg.WebPush.PublicKey = "a-vapid-public-key-value"

	return &di.ApplicationComponents{Infra: &di.InfraModule{PushSubscriptionGateway: pushGateway}},
		cfg,
		slog.New(slog.NewTextHandler(io_Discard{}, nil))
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
	SetupOperatorConnectHandlers(mux, container, cfg, logger, "test-operator-token", true)

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
	SetupOperatorConnectHandlers(operatorMux, container, cfg, logger, "test-operator-token", true)

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

// TestSetupOperatorConnectHandlers_RejectsUnauthenticatedRequests proves the
// operator auth interceptor is actually wired onto the admin handlers, not
// merely unit-tested in isolation (CLAUDE.md rule 8: check the interceptor
// reaches the handler chain, not just that it exists). Both cases reject
// before the handler runs, so they are safe to exercise even though testDeps
// wires every admin usecase to nil — a request that passed auth would panic
// on the nil usecase, which is exactly why this test only covers the reject
// path; the accept path is covered at the interceptor unit level in
// connect/v2/middleware.
func TestSetupOperatorConnectHandlers_RejectsUnauthenticatedRequests(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupOperatorConnectHandlers(mux, container, cfg, logger, "correct-operator-token", true)

	path := "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth"

	t.Run("missing Authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("wrong bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

// TestSetupOperatorConnectHandlers_Disabled_SkipsAuthCheck proves
// OPERATOR_AUTH=disabled reaches the interceptor as enabled=false rather than
// silently defaulting to it. It cannot assert a 200 for the reason described
// above (nil usecase), so it asserts the request was not rejected for lack of
// auth — an unauthenticated 401 here would mean "disabled" failed to travel
// from config to the interceptor.
func TestSetupOperatorConnectHandlers_Disabled_SkipsAuthCheck(t *testing.T) {
	container, cfg, logger := testDeps()
	mux := http.NewServeMux()
	SetupOperatorConnectHandlers(mux, container, cfg, logger, "", false)

	req := httptest.NewRequest(http.MethodPost,
		"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	defer func() {
		// A panic here means the request reached the nil usecase, i.e. auth
		// was correctly skipped; that is this test passing, not failing.
		if r := recover(); r != nil {
			return
		}
	}()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Errorf("status = %d, want anything but 401 when OPERATOR_AUTH=disabled", rec.Code)
	}
}
