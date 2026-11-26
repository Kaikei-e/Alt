package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"inference-proxy/internal/httpserver"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

const (
	goldenDirMode  os.FileMode = 0o755
	goldenFileMode os.FileMode = 0o644
	goldenDirPath              = "testdata"
	goldenFilePath             = "testdata/effective_config.golden.json"
)

type inferenceProxyEffectiveConfig struct {
	BodyLimitBoundary        bodyBoundarySnapshot     `json:"body_limit_boundary"`
	DefaultMaxBodyBytes      int64                    `json:"default_max_request_body_bytes"`
	RoutePolicy              map[string]int           `json:"route_policy_status"`
	Server                   serverTimeout            `json:"server"`
	Service                  string                   `json:"service"`
	UpstreamForwardedHeaders upstreamForwardedHeaders `json:"upstream_forwarded_headers"`
}

type bodyBoundarySnapshot struct {
	AtLimitStatus   int   `json:"at_limit_status"`
	LimitBytes      int64 `json:"limit_bytes"`
	OverLimitStatus int   `json:"over_limit_status"`
	OverLimitUpHits int64 `json:"over_limit_upstream_hits"`
}

type upstreamForwardedHeaders struct {
	AuthorizationStripped bool   `json:"authorization_stripped"`
	CustomHeader          string `json:"custom_header"`
	Forwarded             string `json:"forwarded"`
	HostMatchesTarget     bool   `json:"host_matches_target"`
	PathAndQuery          string `json:"path_and_query"`
	XForwardedFor         string `json:"x_forwarded_for"`
	XForwardedHost        string `json:"x_forwarded_host"`
	XForwardedProto       string `json:"x_forwarded_proto"`
}

type serverTimeout struct {
	Addr              string `json:"addr"`
	IdleTimeout       string `json:"idle_timeout"`
	MaxHeaderBytes    int    `json:"max_header_bytes"`
	ReadHeaderTimeout string `json:"read_header_timeout"`
	ReadTimeout       string `json:"read_timeout"`
	WriteTimeout      string `json:"write_timeout"`
}

func TestEffectiveConfigGolden(t *testing.T) {
	snap := captureInferenceProxyEffectiveConfig(t, nil, nil)
	got := mustCanonicalJSON(t, snap)

	if *updateGolden || os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(goldenDirPath, goldenDirMode); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenFilePath, got, goldenFileMode); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update or UPDATE_GOLDEN=1 to create): %v", goldenFilePath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("effective runtime config drifted from %s:\n%s", goldenFilePath, diffLines(string(want), string(got)))
	}
}

func TestEffectiveConfigGolden_MutationsDetected(t *testing.T) {
	want, err := os.ReadFile(goldenFilePath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenFilePath, err)
	}

	t.Run("C-14 mutation using naive Rewrite+SetXForwarded alters X-Forwarded-For and produces golden diff", func(t *testing.T) {
		naiveHandlerBuilder := func(targetURL *url.URL, _ string) http.Handler {
			// C-14 anti-pattern: Rewrite with pr.SetXForwarded() drops inbound
			// X-Forwarded-For and synthesizes X-Forwarded-Host / X-Forwarded-Proto.
			rp := &httputil.ReverseProxy{
				FlushInterval: -1,
				Rewrite: func(pr *httputil.ProxyRequest) {
					pr.SetURL(targetURL)
					pr.SetXForwarded()
					pr.Out.Host = targetURL.Host
					pr.Out.Header.Del("Authorization")
				},
			}
			mux := http.NewServeMux()
			mux.Handle("/", rp)
			return mux
		}
		snap := captureInferenceProxyEffectiveConfig(t, naiveHandlerBuilder, nil)
		if snap.UpstreamForwardedHeaders.XForwardedFor == "203.0.113.10, 127.0.0.1" {
			t.Fatalf("expected C-14 naive SetXForwarded to drop inbound X-Forwarded-For, got %q", snap.UpstreamForwardedHeaders.XForwardedFor)
		}
		if snap.UpstreamForwardedHeaders.XForwardedHost == "" || snap.UpstreamForwardedHeaders.XForwardedProto == "" {
			t.Fatalf("expected C-14 naive SetXForwarded to synthesize X-Forwarded-Host/Proto, got host=%q proto=%q",
				snap.UpstreamForwardedHeaders.XForwardedHost, snap.UpstreamForwardedHeaders.XForwardedProto)
		}
		got := mustCanonicalJSON(t, snap)
		if bytes.Equal(got, want) {
			t.Fatal("C-14 SetXForwarded mutation matched golden; expected diff")
		}
	})

	t.Run("C-11 style IdleTimeout mutation produces golden diff", func(t *testing.T) {
		mutatedSrv := func(addr string, h http.Handler) *http.Server {
			return httpserver.New(httpserver.Config{
				Addr:              addr,
				Handler:           h,
				ReadHeaderTimeout: DefaultReadHeaderTimeout,
				ReadTimeout:       DefaultReadTimeout,
				WriteTimeout:      0,
				IdleTimeout:       60 * time.Second,
			})
		}
		got := mustCanonicalJSON(t, captureInferenceProxyEffectiveConfig(t, nil, mutatedSrv))
		if bytes.Equal(got, want) {
			t.Fatal("IdleTimeout mutation matched golden; expected diff")
		}
	})
}

func captureInferenceProxyEffectiveConfig(
	t *testing.T,
	customProxyBuilder func(*url.URL, string) http.Handler,
	customServerBuilder func(string, http.Handler) *http.Server,
) inferenceProxyEffectiveConfig {
	t.Helper()
	const expectedToken = "golden-inference-token"

	var captured upstreamForwardedHeaders
	var targetHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = upstreamForwardedHeaders{
			AuthorizationStripped: r.Header.Get("Authorization") == "",
			CustomHeader:          r.Header.Get("X-Custom-Header"),
			Forwarded:             r.Header.Get("Forwarded"),
			HostMatchesTarget:     r.Host == targetHost,
			PathAndQuery:          r.URL.RequestURI(),
			XForwardedFor:         r.Header.Get("X-Forwarded-For"),
			XForwardedHost:        r.Header.Get("X-Forwarded-Host"),
			XForwardedProto:       r.Header.Get("X-Forwarded-Proto"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	targetHost = targetURL.Host

	var handler http.Handler
	if customProxyBuilder != nil {
		handler = customProxyBuilder(targetURL, expectedToken)
	} else {
		handler = NewProxyHandler(targetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)
	}
	proxySrv := httptest.NewServer(handler)
	defer proxySrv.Close()

	// 1. Live probe of all 6+ upstream forwarded headers (guarding C-14).
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxySrv.URL+"/api/generate?stream=true&model=qwen", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("build probe request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("Forwarded", "for=192.0.2.60")
	req.Header.Set("X-Custom-Header", "custom-val")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe request status = %d, want 200", resp.StatusCode)
	}

	// 2. Route allowlist/denylist probe on real NewProxyHandler with its own upstream
	// so step 1's captured forwarding headers are never overwritten.
	routeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer routeUpstream.Close()
	routeTargetURL, err := url.Parse(routeUpstream.URL)
	if err != nil {
		t.Fatalf("parse route upstream URL: %v", err)
	}
	realHandler := NewProxyHandler(routeTargetURL, expectedToken, 5*time.Second, DefaultMaxRequestBodyBytes)
	realProxySrv := httptest.NewServer(realHandler)
	defer realProxySrv.Close()

	routePolicy := map[string]int{}
	for _, tc := range []struct {
		key    string
		method string
		path   string
		auth   bool
	}{
		{"GET /health (unauth)", http.MethodGet, "/health", false},
		{"GET /api/tags (unauth)", http.MethodGet, "/api/tags", false},
		{"GET /api/tags", http.MethodGet, "/api/tags", true},
		{"GET /api/ps", http.MethodGet, "/api/ps", true},
		{"POST /api/generate", http.MethodPost, "/api/generate", true},
		{"POST /api/chat", http.MethodPost, "/api/chat", true},
		{"POST /api/embed", http.MethodPost, "/api/embed", true},
		{"POST /api/embeddings", http.MethodPost, "/api/embeddings", true},
		{"POST /api/pull", http.MethodPost, "/api/pull", true},
		{"DELETE /api/delete", http.MethodDelete, "/api/delete", true},
	} {
		r, err := http.NewRequestWithContext(t.Context(), tc.method, realProxySrv.URL+tc.path, bytes.NewReader([]byte(`{}`)))
		if err != nil {
			t.Fatalf("build route probe %s: %v", tc.key, err)
		}
		if tc.auth {
			r.Header.Set("Authorization", "Bearer "+expectedToken)
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatalf("do route probe %s: %v", tc.key, err)
		}
		_ = res.Body.Close()
		routePolicy[tc.key] = res.StatusCode
	}

	// 3. Body boundary probe on a 1024-byte instance of NewProxyHandler.
	var bodyHits atomic.Int64
	bodyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyHits.Add(1)
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer bodyUpstream.Close()
	bodyURL, _ := url.Parse(bodyUpstream.URL)
	const probeLimit int64 = 1024
	bodyProxy := httptest.NewServer(NewProxyHandler(bodyURL, expectedToken, 5*time.Second, probeLimit))
	defer bodyProxy.Close()

	atReq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, bodyProxy.URL+"/api/generate", bytes.NewReader(bytes.Repeat([]byte("a"), int(probeLimit))))
	atReq.Header.Set("Authorization", "Bearer "+expectedToken)
	atResp, err := http.DefaultClient.Do(atReq)
	if err != nil {
		t.Fatalf("at-limit request: %v", err)
	}
	_ = atResp.Body.Close()

	bodyHits.Store(0)
	overReq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, bodyProxy.URL+"/api/generate", bytes.NewReader(bytes.Repeat([]byte("b"), int(probeLimit)+1)))
	overReq.Header.Set("Authorization", "Bearer "+expectedToken)
	overResp, err := http.DefaultClient.Do(overReq)
	if err != nil {
		t.Fatalf("over-limit request: %v", err)
	}
	_ = overResp.Body.Close()

	// 4. Construct real HTTP server via NewServer.
	var srv *http.Server
	if customServerBuilder != nil {
		srv = customServerBuilder(":11436", realHandler)
	} else {
		srv = NewServer(":11436", realHandler)
	}

	return inferenceProxyEffectiveConfig{
		Service:             "inference-proxy",
		DefaultMaxBodyBytes: DefaultMaxRequestBodyBytes,
		BodyLimitBoundary: bodyBoundarySnapshot{
			LimitBytes:      probeLimit,
			AtLimitStatus:   atResp.StatusCode,
			OverLimitStatus: overResp.StatusCode,
			OverLimitUpHits: bodyHits.Load(),
		},
		RoutePolicy:              routePolicy,
		UpstreamForwardedHeaders: captured,
		Server: serverTimeout{
			Addr:              srv.Addr,
			ReadHeaderTimeout: srv.ReadHeaderTimeout.String(),
			ReadTimeout:       srv.ReadTimeout.String(),
			WriteTimeout:      srv.WriteTimeout.String(),
			IdleTimeout:       srv.IdleTimeout.String(),
			MaxHeaderBytes:    srv.MaxHeaderBytes,
		},
	}
}

func mustCanonicalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("json.Decode: %v", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(parsed); err != nil {
		t.Fatalf("json.Encode: %v", err)
	}
	return buf.Bytes()
}

func diffLines(want, got string) string {
	wLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	gLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	n := len(wLines)
	if len(gLines) > n {
		n = len(gLines)
	}
	var b strings.Builder
	for i := range n {
		var wl, gl string
		hasW, hasG := i < len(wLines), i < len(gLines)
		if hasW {
			wl = wLines[i]
		}
		if hasG {
			gl = gLines[i]
		}
		if hasW && hasG && wl == gl {
			continue
		}
		if hasW {
			fmt.Fprintf(&b, "- %s\n", wl)
		}
		if hasG {
			fmt.Fprintf(&b, "+ %s\n", gl)
		}
	}
	return b.String()
}
