package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBuildMTLSHandler_RoutesConnectPathsToConnectMux proves that Connect-RPC
// prefixes (/alt.* and /services.*) go to the connect mux.
func TestBuildMTLSHandler_RoutesConnectPathsToConnectMux(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.HandleFunc("/alt.feeds.v2.FeedService/Get", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Source", "connect")
		w.WriteHeader(http.StatusOK)
	})
	connectMux.HandleFunc("/services.backend.v1.BackendInternalService/ListUnsummarizedArticles", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Source", "connect")
		w.WriteHeader(http.StatusOK)
	})

	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Source", "echo")
		w.WriteHeader(http.StatusOK)
	})

	h := buildMTLSHandler(connectMux, echoHandler)

	cases := []struct {
		name       string
		path       string
		wantSource string
	}{
		{"alt prefix goes to connect", "/alt.feeds.v2.FeedService/Get", "connect"},
		{"services prefix goes to connect", "/services.backend.v1.BackendInternalService/ListUnsummarizedArticles", "connect"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("X-Source"); got != tc.wantSource {
				t.Errorf("expected X-Source=%q, got %q", tc.wantSource, got)
			}
		})
	}
}

// TestBuildMTLSHandler_RoutesRESTToEcho proves that non-Connect-RPC paths
// (such as /v1/recap/articles used by recap-worker) fall through to the echo
// handler. This is the fix for the 3days Recap 404: prior to this change the
// mTLS listener wrapped only the connect mux, so REST routes returned 404 on
// :9443 even though they were registered on :9000.
func TestBuildMTLSHandler_RoutesRESTToEcho(t *testing.T) {
	connectMux := http.NewServeMux()
	// connect mux has NO /v1/... handlers registered

	echoCalls := 0
	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		echoCalls++
		if r.URL.Path != "/v1/recap/articles" {
			t.Errorf("expected echo to see /v1/recap/articles, got %q", r.URL.Path)
		}
		w.Header().Set("X-Source", "echo")
		w.WriteHeader(http.StatusOK)
	})

	h := buildMTLSHandler(connectMux, echoHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/recap/articles?from=2026-04-14T00:00:00Z&to=2026-04-15T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from echo, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Source"); got != "echo" {
		t.Errorf("expected X-Source=echo, got %q", got)
	}
	if echoCalls != 1 {
		t.Errorf("expected echo handler to be invoked once, got %d", echoCalls)
	}
}

// TestBuildMTLSHandler_UnmatchedConnectPathDoesNotFallthrough pins down that
// requests with a Connect-RPC prefix but an unknown method get a 404 from the
// connect mux, NOT forwarded to echo. Falling through to echo would hide
// genuine Connect-RPC typos and surface confusing REST errors.
func TestBuildMTLSHandler_UnmatchedConnectPathDoesNotFallthrough(t *testing.T) {
	connectMux := http.NewServeMux()
	connectMux.HandleFunc("/alt.feeds.v2.FeedService/Get", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("echo must not be invoked for unknown Connect-RPC paths")
	})

	h := buildMTLSHandler(connectMux, echoHandler)

	req := httptest.NewRequest(http.MethodPost, "/alt.feeds.v2.FeedService/DoesNotExist", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from connect mux, got %d", rec.Code)
	}
}
