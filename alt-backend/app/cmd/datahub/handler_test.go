package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func sourceHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Source", name)
		w.WriteHeader(http.StatusOK)
	})
}

// data-hub's single listener carries the service-to-service surfaces and
// nothing else. The pre-split mTLS listener also served the whole browser
// REST API and the user Connect services from the same socket; that mix is
// what the split removed, so anything outside the two internal namespaces is
// a 404 here rather than a route into the user API.
func TestDataHubHandler_ServesOnlyTheServiceToServiceSurfaces(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.Handle("/services.backend.v1.BackendInternalService/", sourceHandler("connect"))
	connectMux.Handle("/alt.datahub.v1.DataHubService/", sourceHandler("connect"))

	h := dataHubHandler(connectMux, sourceHandler("internal-echo"))

	tests := []struct {
		path       string
		wantSource string
		wantCode   int
	}{
		// Both namespaces alt-data-hub serves during ADR-000954 Wave 2 reach
		// the connect mux. The legacy row inverts in Wave 2-C.
		{path: "/alt.datahub.v1.DataHubService/CreateArticle", wantSource: "connect", wantCode: http.StatusOK},
		{path: "/alt.datahub.v1.DataHubService/ListRecentArticles", wantSource: "connect", wantCode: http.StatusOK},
		{path: "/services.backend.v1.BackendInternalService/CreateArticle", wantSource: "connect", wantCode: http.StatusOK},
		{path: "/v1/internal/system-user", wantSource: "internal-echo", wantCode: http.StatusOK},
		{path: "/v1/internal/articles/recent?limit=0", wantSource: "internal-echo", wantCode: http.StatusOK},
		{path: "/health", wantSource: "internal-echo", wantCode: http.StatusOK},
		// User-facing REST and Connect surfaces live on cmd/backend now.
		{path: "/v1/feeds", wantSource: "", wantCode: http.StatusNotFound},
		{path: "/v1/recap/articles", wantSource: "", wantCode: http.StatusNotFound},
		{path: "/alt.feeds.v2.FeedService/GetFeedStats", wantSource: "", wantCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
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

// A Connect-RPC path with an unknown method must 404 from the connect mux, not
// fall through to the REST handler: falling through would answer a genuine RPC
// typo with a confusing REST error.
func TestDataHubHandler_UnknownConnectMethodDoesNotFallThrough(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.HandleFunc("/services.backend.v1.BackendInternalService/CreateArticle", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	restHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("the REST handler must not see unknown Connect-RPC paths")
	})

	h := dataHubHandler(connectMux, restHandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/services.backend.v1.BackendInternalService/DoesNotExist", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 from the connect mux", rec.Code)
	}
}
