package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	adapterhandler "auth-hub/internal/adapter/handler"
	"auth-hub/internal/fsperm"
	"auth-hub/internal/httpserver"
	infratoken "auth-hub/internal/infrastructure/token"
	"auth-hub/tlsutil"
	"auth-hub/utils/otel"

	"github.com/labstack/echo/v5"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

type authHubEffectiveConfig struct {
	BodyLimitBytes           int64                    `json:"body_limit_bytes"`
	CORSAllowedOrigins       []string                 `json:"cors_allowed_origins"`
	GlobalMiddlewares        []string                 `json:"global_middlewares"`
	OTelGlobalMiddlewares    []string                 `json:"otel_global_middlewares"`
	PlaintextHealthOnlyCheck map[string]int           `json:"plaintext_health_only_status"`
	Routes                   []authHubRouteSpec       `json:"routes"`
	SecurityHeaders          map[string]string        `json:"security_headers"`
	Servers                  map[string]serverTimeout `json:"servers"`
	Service                  string                   `json:"service"`
}

type authHubRouteSpec struct {
	Method      string   `json:"method"`
	Middlewares []string `json:"middlewares"`
	Path        string   `json:"path"`
}

type serverTimeout struct {
	Addr              string `json:"addr"`
	IdleTimeout       string `json:"idle_timeout"`
	MaxHeaderBytes    int    `json:"max_header_bytes"`
	ReadHeaderTimeout string `json:"read_header_timeout"`
	ReadTimeout       string `json:"read_timeout"`
	WriteTimeout      string `json:"write_timeout"`
}

const (
	goldenDirPath  = "testdata"
	goldenFilePath = "testdata/effective_config.golden.json"
)

func TestEffectiveConfigGolden(t *testing.T) {
	snap := captureAuthHubEffectiveConfig(t, newPlaintextServer)
	got := mustCanonicalJSON(t, snap)

	if *updateGolden || os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(goldenDirPath, fsperm.GeneratedDir); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenFilePath, got, fsperm.GeneratedFile); err != nil {
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

	t.Run("C-11 mutation adding IdleTimeout 60s to plaintext server produces golden diff", func(t *testing.T) {
		mutatedPlaintext := func(addr string, handler http.Handler) *http.Server {
			return httpserver.New(httpserver.Config{
				Addr:              addr,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       60 * time.Second,
			})
		}
		got := mustCanonicalJSON(t, captureAuthHubEffectiveConfig(t, mutatedPlaintext))
		if bytes.Equal(got, want) {
			t.Fatal("C-11 IdleTimeout mutation matched golden; expected diff")
		}
	})

	t.Run("middleware order mutation produces golden diff", func(t *testing.T) {
		snap := captureAuthHubEffectiveConfig(t, newPlaintextServer)
		if len(snap.GlobalMiddlewares) < 2 {
			t.Fatalf("expected at least 2 global middlewares, got %d", len(snap.GlobalMiddlewares))
		}
		snap.GlobalMiddlewares[0], snap.GlobalMiddlewares[1] = snap.GlobalMiddlewares[1], snap.GlobalMiddlewares[0]
		got := mustCanonicalJSON(t, snap)
		if bytes.Equal(got, want) {
			t.Fatal("middleware reorder mutation matched golden; expected diff")
		}
	})
}

func captureAuthHubEffectiveConfig(t *testing.T, buildPlaintext func(string, http.Handler) *http.Server) authHubEffectiveConfig {
	t.Helper()
	cfg := separatedSecretsConfig()
	cfg.TrustedProxiesDisabled = true
	cfg.ValidateRateLimit = 100.0 / 60.0
	cfg.SessionRateLimit = 30.0 / 60.0
	cfg.CSRFRateLimit = 100.0
	cfg.IntrospectRateLimit = 1200.0 / 60.0
	cfg.IntrospectBurst = 100

	log := slog.New(slog.DiscardHandler)
	jwtIssuer := infratoken.NewJWTIssuer(infratoken.JWTConfig{
		PrivateKey: cfg.BackendTokenSigningKey,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})
	handlers := authHubHandlers{
		validate: adapterhandler.NewValidateHandler(nil, jwtIssuer),
		session:  adapterhandler.NewSessionHandler(nil, nil),
		csrf:     adapterhandler.NewCSRFHandler(nil),
		health:   adapterhandler.NewHealthHandler(),
		internal: adapterhandler.NewInternalHandler(nil, nil, nil),
	}

	e := echo.New()
	type rawRoute struct {
		method string
		path   string
		mws    []string
	}
	captured := map[string]rawRoute{}
	e.OnAddRoute = func(r echo.Route) error {
		if r.Method == echo.RouteNotFound {
			return nil
		}
		captured[r.Method+"\x00"+r.Path] = rawRoute{
			method: r.Method,
			path:   r.Path,
			mws:    middlewareNames(r.Middlewares),
		}
		return nil
	}
	configureAuthHubEcho(context.Background(), e, cfg, otel.Config{Enabled: false, ServiceName: "auth-hub"}, log, handlers)

	globalMws := middlewareNames(e.Middlewares())
	routes := make([]authHubRouteSpec, 0, len(captured))
	for _, r := range captured {
		stack := make([]string, 0, len(globalMws)+len(r.mws))
		stack = append(stack, globalMws...)
		stack = append(stack, r.mws...)
		routes = append(routes, authHubRouteSpec{
			Method:      r.method,
			Path:        r.path,
			Middlewares: stack,
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})

	eOTel := echo.New()
	configureAuthHubEcho(context.Background(), eOTel, cfg, otel.Config{Enabled: true, ServiceName: "auth-hub"}, log, handlers)
	otelMws := middlewareNames(eOTel.Middlewares())

	// Runtime probe: plaintext listener permits /health and denies business routes.
	plaintextHandler := buildPlaintextHandler(e, false)
	plaintextChecks := map[string]int{}
	for _, ep := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/session"},
		{http.MethodGet, "/validate"},
		{http.MethodPost, "/csrf"},
		{http.MethodGet, "/internal/system-user"},
	} {
		req := httptest.NewRequestWithContext(t.Context(), ep.method, ep.path, nil)
		rec := httptest.NewRecorder()
		plaintextHandler.ServeHTTP(rec, req)
		plaintextChecks[ep.method+" "+ep.path] = rec.Code
	}

	// Runtime probe: security headers on /health and CORS absence.
	healthReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	healthReq.Header.Set("Origin", "https://evil.example")
	healthRec := httptest.NewRecorder()
	e.ServeHTTP(healthRec, healthReq)

	corsOrigins := []string{}
	if allow := healthRec.Header().Get("Access-Control-Allow-Origin"); allow != "" {
		corsOrigins = append(corsOrigins, allow)
	}
	secHeaders := map[string]string{
		"Content-Security-Policy": healthRec.Header().Get("Content-Security-Policy"),
		"Referrer-Policy":         healthRec.Header().Get("Referrer-Policy"),
		"X-Content-Type-Options":  healthRec.Header().Get("X-Content-Type-Options"),
		"X-Frame-Options":         healthRec.Header().Get("X-Frame-Options"),
	}

	// Real server constructors for plaintext (C-11) and mTLS listeners.
	plaintextSrv := buildPlaintext(":"+cfg.Port, plaintextHandler)
	mtlsSrv := tlsutil.NewMTLSHTTPServer(":9443", nil, e)

	return authHubEffectiveConfig{
		Service:                  "auth-hub",
		BodyLimitBytes:           0,
		CORSAllowedOrigins:       corsOrigins,
		GlobalMiddlewares:        globalMws,
		OTelGlobalMiddlewares:    otelMws,
		PlaintextHealthOnlyCheck: plaintextChecks,
		Routes:                   routes,
		SecurityHeaders:          secHeaders,
		Servers: map[string]serverTimeout{
			"mtls":      extractServerTimeout(mtlsSrv),
			"plaintext": extractServerTimeout(plaintextSrv),
		},
	}
}

func middlewareNames(mws []echo.MiddlewareFunc) []string {
	out := make([]string, 0, len(mws))
	for _, mw := range mws {
		if mw == nil {
			out = append(out, "<nil>")
			continue
		}
		fn := runtime.FuncForPC(reflect.ValueOf(mw).Pointer())
		if fn == nil {
			out = append(out, "<unknown>")
			continue
		}
		out = append(out, fn.Name())
	}
	return out
}

func extractServerTimeout(srv *http.Server) serverTimeout {
	return serverTimeout{
		Addr:              srv.Addr,
		ReadHeaderTimeout: srv.ReadHeaderTimeout.String(),
		ReadTimeout:       srv.ReadTimeout.String(),
		WriteTimeout:      srv.WriteTimeout.String(),
		IdleTimeout:       srv.IdleTimeout.String(),
		MaxHeaderBytes:    srv.MaxHeaderBytes,
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
