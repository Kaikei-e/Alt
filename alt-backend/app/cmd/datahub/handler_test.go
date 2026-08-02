package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/dataplane/connect/datahubapi"
)

func sourceHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Source", name)
		w.WriteHeader(http.StatusOK)
	})
}

// data-hub's single listener carries the service-to-service surface and
// nothing else. The pre-split mTLS listener also served the whole browser
// REST API and the user Connect services from the same socket; that mix is
// what the split removed, so anything outside services.datahub.v1 is a 404 here
// rather than a route into the user API.
func TestDataHubHandler_ServesOnlyTheServiceToServiceSurface(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.Handle("/services.datahub.v1.DataHubService/", sourceHandler("connect"))
	connectMux.Handle("/health", sourceHandler("connect"))

	h := dataHubHandler(connectMux)

	tests := []struct {
		name       string
		path       string
		wantSource string
		wantCode   int
	}{
		{name: "datahub rpc", path: "/services.datahub.v1.DataHubService/CreateArticle", wantSource: "connect", wantCode: http.StatusOK},
		{name: "absorbed rest read", path: "/services.datahub.v1.DataHubService/ListRecentArticles", wantSource: "connect", wantCode: http.StatusOK},
		{name: "health", path: "/health", wantSource: "connect", wantCode: http.StatusOK},

		// ADR-000954 Wave 2-C retired both former names of this surface. A
		// caller still on either one must be told the surface is gone, not
		// handed a second door onto the same data — the point of the deletion
		// is that there is exactly one path to alt-db's owner.
		{name: "retired legacy connect namespace", path: "/services.backend.v1.BackendInternalService/CreateArticle", wantCode: http.StatusNotFound},
		{name: "retired internal rest system-user", path: "/v1/internal/system-user", wantCode: http.StatusNotFound},
		{name: "retired internal rest recent articles", path: "/v1/internal/articles/recent?limit=0", wantCode: http.StatusNotFound},

		// User-facing REST and Connect surfaces live on cmd/backend now.
		{name: "user rest", path: "/v1/feeds", wantCode: http.StatusNotFound},
		{name: "user recap rest", path: "/v1/recap/articles", wantCode: http.StatusNotFound},
		{name: "user connect", path: "/alt.feeds.v2.FeedService/GetFeedStats", wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("X-Source"); got != tt.wantSource {
				t.Errorf("X-Source = %q, want %q", got, tt.wantSource)
			}
		})
	}
}

// A Connect-RPC path with an unknown method must 404 from the connect mux
// rather than from this router: the two answers are indistinguishable to the
// caller, but only the first one means the mux was consulted at all, which is
// what keeps "the procedure was removed" from being reported by a router that
// never learned the procedure existed.
func TestDataHubHandler_UnknownConnectMethodReachesTheConnectMux(t *testing.T) {
	consulted := false
	connectMux := http.NewServeMux()
	connectMux.HandleFunc("/services.datahub.v1.DataHubService/", func(w http.ResponseWriter, _ *http.Request) {
		consulted = true
		w.WriteHeader(http.StatusNotFound)
	})

	h := dataHubHandler(connectMux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/services.datahub.v1.DataHubService/DoesNotExist", nil))

	if !consulted {
		t.Fatal("the connect mux was never consulted for a services.datahub.v1 path")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 from the connect mux", rec.Code)
	}
}

// Transitional coverage for the ADR-000955 big-bang rename: consumers
// deployed before the rename still call alt.datahub.v1.DataHubService, and
// the pact broker keeps selecting their pacts until a renamed version of each
// consumer is recorded as deployed. The shipped handler is therefore wrapped
// in datahubapi.LegacyNamespaceAlias so those callers reach the same connect
// mux instead of a 404 mid-rollout. Delete this test together with the alias
// once every deployed DataHubService consumer publishes pacts on
// services.datahub.v1.
func TestDataHubHandler_LegacyNamespaceAliasServesRetiredDataHubName(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.Handle("/services.datahub.v1.DataHubService/", sourceHandler("connect"))

	h := datahubapi.LegacyNamespaceAlias(dataHubHandler(connectMux))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/alt.datahub.v1.DataHubService/CreateArticle", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy datahub path: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Source"); got != "connect" {
		t.Errorf("legacy datahub path: X-Source = %q, want %q", got, "connect")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/services.backend.v1.BackendInternalService/CreateArticle", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("retired backend namespace: status = %d, want %d through the alias", rec.Code, http.StatusNotFound)
	}
}
