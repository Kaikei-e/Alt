package proxy

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

const (
	goldenDirMode  os.FileMode = 0o755
	goldenFileMode os.FileMode = 0o644
	goldenDirPath              = "testdata"
	goldenFilePath             = "testdata/effective_config.golden.json"
)

type sidecarEffectiveConfig struct {
	ErrorResponse errorResponseSnapshot `json:"error_response"`
	ProxySettings proxySettingsSnapshot `json:"proxy_settings"`
	Server        serverTimeout         `json:"server"`
	Service       string                `json:"service"`
}

type errorResponseSnapshot struct {
	Body                string `json:"body"`
	ContentType         string `json:"content_type"`
	Status              int    `json:"status"`
	XContentTypeOptions string `json:"x_content_type_options"`
	XTraceID            string `json:"x_trace_id"`
}

type proxySettingsSnapshot struct {
	AllowedDomains       []string `json:"allowed_domains"`
	AutoLearnEnabled     bool     `json:"autolearn_enabled"`
	BufferSizeBytes      int      `json:"buffer_size_bytes"`
	ConnectClientTimeout string   `json:"connect_client_timeout"`
	ConnectTimeout       string   `json:"connect_timeout"`
	DNSCacheTimeout      string   `json:"dns_cache_timeout"`
	DNSServers           []string `json:"dns_servers"`
	DNSTimeout           string   `json:"dns_timeout"`
	MaxConcurrentReqs    int      `json:"max_concurrent_reqs"`
	MaxRequestSizeBytes  int64    `json:"max_request_size_bytes"`
	RequestTimeout       string   `json:"request_timeout"`
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
	cfg := loadDefaultSidecarConfig(t)
	snap := captureSidecarEffectiveConfig(t, cfg, false)
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

	t.Run("C-13 mutation HTML-escaping plain-text error body produces golden diff", func(t *testing.T) {
		cfg := loadDefaultSidecarConfig(t)
		got := mustCanonicalJSON(t, captureSidecarEffectiveConfig(t, cfg, true))
		if bytes.Equal(got, want) {
			t.Fatal("C-13 HTML-escaped error body mutation matched golden; expected diff")
		}
	})

	t.Run("C-11 style IdleTimeout mutation produces golden diff", func(t *testing.T) {
		cfg := loadDefaultSidecarConfig(t)
		cfg.IdleTimeout = 60 * time.Second
		got := mustCanonicalJSON(t, captureSidecarEffectiveConfig(t, cfg, false))
		if bytes.Equal(got, want) {
			t.Fatal("IdleTimeout mutation matched golden; expected diff")
		}
	})
}

func loadDefaultSidecarConfig(t *testing.T) *config.ProxyConfig {
	t.Helper()
	for _, k := range []string{
		"LISTEN_PORT",
		"REQUEST_TIMEOUT",
		"READ_TIMEOUT",
		"WRITE_TIMEOUT",
		"IDLE_TIMEOUT",
		"MAX_RETRIES",
		"ENVOY_UPSTREAM",
		"ENVOY_MAX_CONNS",
		"ENVOY_MAX_IDLE_CONNS",
		"DNS_SERVERS",
		"DNS_TIMEOUT",
		"DNS_CACHE_TIMEOUT",
		"DNS_MAX_CACHE_ENTRIES",
		"ALLOWED_DOMAINS",
		"MAX_REQUEST_SIZE",
		"HEADER_TIMEOUT_SEC",
		"BUFFER_SIZE",
		"MAX_CONCURRENT_REQS",
		"CONNECT_TIMEOUT",
		"CONNECT_MAX_CONNS",
		"CONNECT_IDLE_TIMEOUT",
		"ENABLE_CONNECT",
	} {
		t.Setenv(k, "")
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("config.LoadConfig(): %v", err)
	}
	return cfg
}

func captureSidecarEffectiveConfig(t *testing.T, cfg *config.ProxyConfig, simulateC13Escape bool) sidecarEffectiveConfig {
	t.Helper()
	p, err := NewLightweightProxy(cfg)
	if err != nil {
		t.Fatalf("NewLightweightProxy(): %v", err)
	}
	srv := p.buildServer()

	// Exercise the real writeErrorResponse with characters (<, >, &, ") that
	// html.EscapeString would corrupt (C-13 regression guard).
	const probeMsg = `upstream <err> & "quote"`
	const probeTrace = "trace-golden-1"
	rec := httptest.NewRecorder()
	if simulateC13Escape {
		p.writeErrorResponse(rec, http.StatusBadGateway, html.EscapeString(probeMsg), html.EscapeString(probeTrace))
	} else {
		p.writeErrorResponse(rec, http.StatusBadGateway, probeMsg, probeTrace)
	}

	domains := append([]string(nil), cfg.AllowedDomainsRaw...)
	sort.Strings(domains)
	dnsServers := append([]string(nil), cfg.DNSServers...)

	return sidecarEffectiveConfig{
		Service: "sidecar-proxy",
		Server: serverTimeout{
			Addr:              srv.Addr,
			ReadHeaderTimeout: srv.ReadHeaderTimeout.String(),
			ReadTimeout:       srv.ReadTimeout.String(),
			WriteTimeout:      srv.WriteTimeout.String(),
			IdleTimeout:       srv.IdleTimeout.String(),
			MaxHeaderBytes:    srv.MaxHeaderBytes,
		},
		ErrorResponse: errorResponseSnapshot{
			Status:              rec.Code,
			ContentType:         rec.Header().Get("Content-Type"),
			XContentTypeOptions: rec.Header().Get("X-Content-Type-Options"),
			XTraceID:            rec.Header().Get("X-Trace-ID"),
			Body:                rec.Body.String(),
		},
		ProxySettings: proxySettingsSnapshot{
			AllowedDomains:       domains,
			AutoLearnEnabled:     p.autoLearner.IsLearningEnabled(),
			BufferSizeBytes:      cfg.BufferSize,
			ConnectClientTimeout: p.connectHTTPClient.Timeout.String(),
			ConnectTimeout:       cfg.CONNECTTimeout.String(),
			DNSCacheTimeout:      cfg.DNSCacheTimeout.String(),
			DNSServers:           dnsServers,
			DNSTimeout:           cfg.DNSTimeout.String(),
			MaxConcurrentReqs:    cfg.MaxConcurrentReqs,
			MaxRequestSizeBytes:  cfg.MaxRequestSize,
			RequestTimeout:       p.httpClient.Timeout.String(),
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
