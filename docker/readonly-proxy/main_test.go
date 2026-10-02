package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		{"GET", "/info", true},
		{"GET", "/version", true},
		{"GET", "/events", true},
		{"GET", "/containers/json", true},
		{"GET", "/containers/123abc_/json", true},
		{"GET", "/containers/test-container.name/stats", true},
		{"GET", "/containers/id123/logs", true},

		// Allowed routes (versioned)
		{"GET", "/v1.41/info", true},
		{"GET", "/v1.44/containers/json", true},
		{"GET", "/v1.44/containers/abc/stats", true},

		// Disallowed HTTP methods
		{"POST", "/containers/json", false},
		{"DELETE", "/containers/abc/json", false},
		{"PUT", "/info", false},
		{"PATCH", "/containers/abc/json", false},
		{"POST", "/v1.44/containers/abc/start", false},

		// Disallowed routes
		{"GET", "/containers/abc/export", false},
		{"GET", "/containers/abc/archive", false},
		{"GET", "/containers/abc/exec", false},
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

	proxyHandler := newProxyHandler(upstream.URL, upstreamURL, upstream.Client().Transport)
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
		{"AllowedInfo", "GET", "/info", http.StatusOK, true},
		{"AllowedVersion", "GET", "/v1.44/version", http.StatusOK, true},
		{"AllowedEvents", "GET", "/events", http.StatusOK, true},
		{"AllowedContainersList", "GET", "/containers/json", http.StatusOK, true},
		{"AllowedContainerStats", "GET", "/v1.44/containers/test-container/stats", http.StatusOK, true},
		{"AllowedContainerLogs", "GET", "/containers/test-container/logs", http.StatusOK, true},

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
