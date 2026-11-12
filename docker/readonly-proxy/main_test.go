package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIsAllowed(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		// Docker engine GET /_ping exact safe compat
		{"GET", "/_ping", true},
		{"GET", "/v1.41/_ping", true},
		{"GET", "/v1.44/_ping", true},

		// Allowed routes (versionless)
		{"GET", "/containers/json", true},
		{"GET", "/containers/id123/logs", true},

		// Allowed routes (versioned)
		{"GET", "/v1.44/containers/json", true},
		{"GET", "/v1.45/containers/json", true},
		{"GET", "/v1.45/containers/abc/logs", true},

		// Disallowed HTTP methods
		{"POST", "/containers/json", false},
		{"DELETE", "/containers/abc/json", false},
		{"PUT", "/info", false},
		{"PATCH", "/containers/abc/json", false},
		{"POST", "/v1.44/containers/abc/start", false},

		// Disallowed routes (including secret-exposing container inspect and events)
		{"GET", "/info", false},
		{"GET", "/version", false},
		{"GET", "/events", false},
		{"GET", "/v1.45/events", false},
		{"GET", "/containers/123abc_/json", false},
		{"GET", "/v1.45/containers/abc/json", false},
		{"GET", "/containers/test-container.name/stats", false},
		{"GET", "/v1.41/info", false},
		{"GET", "/v1.44/containers/abc/stats", false},
		{"GET", "/containers/abc/export", false},
		{"GET", "/containers/abc/archive", false},
		{"GET", "/containers/abc/exec", false},
		{"GET", "/v1.45/containers/abc/exec", false},
		{"GET", "/images/json", false},
		{"GET", "/networks", false},
		{"GET", "/", false},
		{"GET", "/v1.41/images/json", false},
		{"GET", "/v1.44/containers/abc/export", false},
		{"GET", "/v1.44/containers/abc/archive", false},

		// Path traversal and encoded bypasses
		{"GET", "/containers/../info", false},
		{"GET", "/v1.44/containers/..%2Finfo", false},
		{"GET", "/v1.44/containers/abc%2farchive", false},
		{"GET", "/v1.44/containers/abc%2Fexport", false},
		{"GET", "//_ping", false},
		{"GET", "/v1.44//version", false},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		got := isAllowed(req)
		if got != tt.want {
			t.Errorf("isAllowed(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

// TestWireProxyForwardHits verifies that requests are actually forwarded to the upstream
// HTTP daemon only when allowed, and forbidden requests NEVER hit the upstream.
func TestWireProxyForwardHits(t *testing.T) {
	var upstreamHits int64

	// Fake upstream Docker daemon
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&upstreamHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("failed to parse upstream url: %v", err)
	}

	proxyHandler := newProxyHandler(upstreamURL, upstream.Client().Transport, "alt")
	proxyServer := httptest.NewServer(proxyHandler)
	defer proxyServer.Close()

	client := proxyServer.Client()

	tests := []struct {
		name          string
		method        string
		path          string
		wantStatus    int
		shouldForward bool
	}{
		// Allowed endpoints must forward to upstream (upstream hit count +1)
		{"AllowedPing", "GET", "/_ping", http.StatusOK, true},
		{"AllowedHeadPing", "HEAD", "/_ping", http.StatusOK, true},
		{"AllowedVersionedPing", "GET", "/v1.44/_ping", http.StatusOK, true},
		{"AllowedHeadVersionedPing", "HEAD", "/v1.44/_ping", http.StatusOK, true},
		{"AllowedContainersList", "GET", "/containers/json", http.StatusOK, true},
		{"AllowedVersionedContainersList", "GET", "/v1.45/containers/json", http.StatusOK, true},

		// Denied secret-exposing inspect/events endpoints (must NOT forward, status 403)
		{"DenyContainerInspect", "GET", "/v1.45/containers/abc/json", http.StatusForbidden, false},
		{"DenyVersionlessContainerInspect", "GET", "/containers/abc/json", http.StatusForbidden, false},
		{"DenyEvents", "GET", "/events", http.StatusForbidden, false},
		{"DenyVersionedEvents", "GET", "/v1.45/events", http.StatusForbidden, false},
		{"DenyVersionedContainerExec", "GET", "/v1.45/containers/abc/exec", http.StatusForbidden, false},
		{"DenyInfo", "GET", "/info", http.StatusForbidden, false},
		{"DenyVersion", "GET", "/v1.44/version", http.StatusForbidden, false},
		{"DenyContainerStats", "GET", "/v1.44/containers/test-container/stats", http.StatusForbidden, false},

		// Denied mutating operations (must NOT forward, status 403)
		{"DenyPostStart", "POST", "/v1.44/containers/c123/start", http.StatusForbidden, false},
		{"DenyPostContainers", "POST", "/containers/json", http.StatusForbidden, false},
		{"DenyDeleteContainer", "DELETE", "/containers/c123", http.StatusForbidden, false},
		{"DenyPutInfo", "PUT", "/info", http.StatusForbidden, false},

		// Denied dangerous read/export/archive/exec endpoints (must NOT forward, status 403)
		{"DenyContainerArchive", "GET", "/containers/c123/archive", http.StatusForbidden, false},
		{"DenyHeadContainerArchive", "HEAD", "/containers/c123/archive", http.StatusForbidden, false},
		{"DenyContainerExport", "GET", "/containers/c123/export", http.StatusForbidden, false},
		{"DenyHeadContainerExport", "HEAD", "/containers/c123/export", http.StatusForbidden, false},
		{"DenyContainerExec", "GET", "/containers/c123/exec", http.StatusForbidden, false},
		{"DenyVersionedArchive", "GET", "/v1.44/containers/c123/archive", http.StatusForbidden, false},
		{"DenyVersionedExport", "GET", "/v1.44/containers/c123/export", http.StatusForbidden, false},
		{"DenyImagesList", "GET", "/images/json", http.StatusForbidden, false},
		{"DenyHeadImagesList", "HEAD", "/images/json", http.StatusForbidden, false},
		{"DenyNetworksList", "GET", "/networks", http.StatusForbidden, false},
		{"DenyVolumesList", "GET", "/volumes", http.StatusForbidden, false},

		// Traversal / encoded bypass attempts (must NOT forward, status 403)
		{"DenyDotTraversal", "GET", "/containers/../info", http.StatusForbidden, false},
		{"DenyEncodedSlashArchive", "GET", "/v1.44/containers/c123%2farchive", http.StatusForbidden, false},
		{"DenyDoubleSlash", "GET", "//_ping", http.StatusForbidden, false},
		{"DenyDoubleSlashVersion", "GET", "/v1.44//version", http.StatusForbidden, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hitsBefore := atomic.LoadInt64(&upstreamHits)

			req, err := http.NewRequest(tt.method, proxyServer.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("client request failed: %v", err)
			}
			_ = resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d", tt.method, tt.path, resp.StatusCode, tt.wantStatus)
			}

			hitsAfter := atomic.LoadInt64(&upstreamHits)
			diff := hitsAfter - hitsBefore

			if tt.shouldForward && diff != 1 {
				t.Errorf("%s %s expected 1 upstream forward hit, got %d", tt.method, tt.path, diff)
			} else if !tt.shouldForward && diff != 0 {
				t.Errorf("%s %s expected 0 upstream forward hits (blocked), got %d (LEAKED TO UPSTREAM!)", tt.method, tt.path, diff)
			}
		})
	}
}

const (
	scopedID   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	foreignID  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	unlabelled = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

type recordedRequest struct {
	path  string
	query url.Values
}

func newScopedFixture(t *testing.T) (*httptest.Server, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, recordedRequest{path: r.URL.Path, query: r.URL.Query()})
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			var f map[string][]string
			_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &f)
			hasProject := false
			hasGroup := false
			for _, l := range f["label"] {
				if l == "com.docker.compose.project=alt" {
					hasProject = true
				}
				if l == "rask.group" {
					hasGroup = true
				}
			}
			out := []map[string]string{}
			if hasProject && hasGroup {
				for _, id := range f["id"] {
					if id == scopedID {
						out = append(out, map[string]string{"Id": scopedID})
					}
				}
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		_, _ = fmt.Fprint(w, "stream")
	}))
	t.Cleanup(upstream.Close)

	return upstream, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), seen...)
	}
}

func TestLogsAreScopedToProjectAndOptedInContainers(t *testing.T) {
	upstream, seen := newScopedFixture(t)
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(newProxyHandler(upstreamURL, upstream.Client().Transport, "alt"))
	defer proxy.Close()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantLogHit bool
	}{
		{"ScopedContainer", "/containers/" + scopedID + "/logs?stdout=1", http.StatusOK, true},
		{"ScopedContainerVersioned", "/v1.45/containers/" + scopedID + "/logs", http.StatusOK, true},
		{"ForeignContainer", "/containers/" + foreignID + "/logs", http.StatusForbidden, false},
		{"UnlabelledContainer", "/v1.45/containers/" + unlabelled + "/logs", http.StatusForbidden, false},
		{"ContainerNameInsteadOfID", "/containers/alt-backend/logs", http.StatusForbidden, false},
		{"IDPrefix", "/containers/" + scopedID[:12] + "/logs", http.StatusForbidden, false},
		{"UppercaseID", "/containers/" + strings.ToUpper(scopedID) + "/logs", http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(seen())
			resp, err := http.Get(proxy.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			logHit := false
			for _, r := range seen()[before:] {
				if strings.HasSuffix(r.path, "/logs") {
					logHit = true
				}
			}
			if logHit != tt.wantLogHit {
				t.Errorf("logs reached upstream = %v, want %v", logHit, tt.wantLogHit)
			}
		})
	}
}

func TestContainerListIsForcedToProject(t *testing.T) {
	upstream, seen := newScopedFixture(t)
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(newProxyHandler(upstreamURL, upstream.Client().Transport, "alt"))
	defer proxy.Close()

	spoof := url.Values{"filters": {`{"label":["com.docker.compose.project=other"]}`}, "all": {"1"}}
	resp, err := http.Get(proxy.URL + "/containers/json?" + spoof.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	reqs := seen()
	if len(reqs) != 1 {
		t.Fatalf("got %d upstream requests, want 1", len(reqs))
	}
	var f map[string][]string
	if err := json.Unmarshal([]byte(reqs[0].query.Get("filters")), &f); err != nil {
		t.Fatalf("filters not JSON: %v", err)
	}
	labels := strings.Join(f["label"], ",")
	if !strings.Contains(labels, "com.docker.compose.project=alt") {
		t.Errorf("project label filter missing: %v", f)
	}
	if strings.Contains(labels, "project=other") {
		t.Errorf("client-supplied project filter survived: %v", f)
	}
	if reqs[0].query.Get("all") != "1" {
		t.Errorf("other query params must be preserved")
	}
}

func TestHealthcheck(t *testing.T) {
	// 1. Running against an HTTP server that responds 200 OK to /_ping -> exit code 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "OK")
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	hostPort := strings.TrimPrefix(server.URL, "http://")
	code := runHealthcheck(hostPort)
	if code != 0 {
		t.Errorf("runHealthcheck on healthy server got %d, want 0", code)
	}

	// 2. Running against an unreachable address -> exit code 1
	badCode := runHealthcheck("127.0.0.1:59999")
	if badCode != 1 {
		t.Errorf("runHealthcheck on unreachable server got %d, want 1", badCode)
	}
}

func TestLogsAllowedFailsClosed(t *testing.T) {
	base := func(h http.HandlerFunc) (*url.URL, *http.Client) {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		return u, srv.Client()
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL, _ := url.Parse(closed.URL)
	closed.Close()

	okURL, okClient := base(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, scopedID)
	})
	errURL, errClient := base(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, scopedID)
	})
	badURL, badClient := base(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not json")
	})
	otherURL, otherClient := base(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, foreignID)
	})

	tests := []struct {
		name   string
		base   *url.URL
		client *http.Client
		want   bool
	}{
		{"Match", okURL, okClient, true},
		{"DaemonUnreachable", closedURL, http.DefaultClient, false},
		{"Non200", errURL, errClient, false},
		{"UndecodableBody", badURL, badClient, false},
		{"DifferentIDReturned", otherURL, otherClient, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logsAllowed(tt.client, tt.base, scopedID, "alt"); got != tt.want {
				t.Errorf("logsAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogsRefusedWhenLookupFails(t *testing.T) {
	var logHits int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			atomic.AddInt64(&logHits, 1)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(newProxyHandler(upstreamURL, upstream.Client().Transport, "alt"))
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/containers/" + scopedID + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || atomic.LoadInt64(&logHits) != 0 {
		t.Errorf("status %d, log hits %d; want 403 and 0", resp.StatusCode, logHits)
	}
}

func TestProjectFromEnv(t *testing.T) {
	tests := []struct {
		value   string
		want    string
		wantErr bool
	}{
		{"alt", "alt", false},
		{"", "", true},
		{"   ", "", true},
	}
	for _, tt := range tests {
		got, err := projectFromEnv(func(string) string { return tt.value })
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("projectFromEnv(%q) = %q, %v", tt.value, got, err)
		}
	}
}

func TestNewServerSetsReadHeaderTimeoutOnly(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout must be set")
	}
	if srv.WriteTimeout != 0 {
		t.Error("WriteTimeout must stay unset so log follow streams survive")
	}
}
