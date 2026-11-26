package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"alt/config"
	connectmiddleware "alt/connect/v2/middleware"
	"alt/di"
	"alt/internal/bootstrap"
	"alt/internal/fsperm"
	"alt/orchestrator/rest"
	"alt/orchestrator/usecase/csrf_token_usecase"
	"alt/utils/logger"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

type effectiveConfigSnapshot struct {
	BodyLimitBytes        int64                    `json:"body_limit_bytes"`
	CORS                  corsSnapshot             `json:"cors"`
	GlobalMiddlewares     []string                 `json:"global_middlewares"`
	OTelGlobalMiddlewares []string                 `json:"otel_global_middlewares"`
	Routes                []routeSnapshot          `json:"routes"`
	SecurityHeaders       map[string]string        `json:"security_headers"`
	Servers               map[string]serverTimeout `json:"servers"`
	Service               string                   `json:"service"`
}

type corsSnapshot struct {
	AllowedOrigins           []string          `json:"allowed_origins"`
	EmptyOriginsStartupError string            `json:"empty_origins_startup_error"`
	PreflightResults         map[string]string `json:"preflight_results"`
}

type routeSnapshot struct {
	Method      string   `json:"method"`
	Middlewares []string `json:"middlewares"`
	Path        string   `json:"path"`
}

type serverTimeout struct {
	Addr              string   `json:"addr"`
	IdleTimeout       string   `json:"idle_timeout"`
	MaxHeaderBytes    int      `json:"max_header_bytes"`
	Protocols         []string `json:"protocols"`
	ReadHeaderTimeout string   `json:"read_header_timeout"`
	ReadTimeout       string   `json:"read_timeout"`
	WriteTimeout      string   `json:"write_timeout"`
}

type goldenStubCSRFGateway struct{}

func (goldenStubCSRFGateway) GenerateToken(context.Context) (string, error) {
	return "golden-csrf", nil
}
func (goldenStubCSRFGateway) ValidateToken(context.Context, string) (bool, error) { return true, nil }
func (goldenStubCSRFGateway) InvalidateToken(context.Context, string) error       { return nil }

const (
	goldenDirPath  = "testdata"
	goldenFilePath = "testdata/effective_config.golden.json"
)

func TestEffectiveConfigGolden(t *testing.T) {
	cfg := loadCanonicalDefaultBackendConfig(t)
	snap := captureBackendEffectiveConfig(t, cfg, 0)
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

	t.Run("C-11 style IdleTimeout mutation produces golden diff", func(t *testing.T) {
		cfg := loadCanonicalDefaultBackendConfig(t)
		cfg.Server.IdleTimeout = 60 * time.Second
		got := mustCanonicalJSON(t, captureBackendEffectiveConfig(t, cfg, 0))
		if bytes.Equal(got, want) {
			t.Fatal("mutated IdleTimeout matched golden; expected diff")
		}
	})

	t.Run("C-15 style BodyLimit mutation produces golden diff", func(t *testing.T) {
		cfg := loadCanonicalDefaultBackendConfig(t)
		const mutatedBodyLimitBytes int64 = 1_000_000
		got := mustCanonicalJSON(t, captureBackendEffectiveConfig(t, cfg, mutatedBodyLimitBytes))
		if bytes.Equal(got, want) {
			t.Fatal("mutated BodyLimit matched golden; expected diff")
		}
	})

	t.Run("CORS allowed origins mutation produces golden diff", func(t *testing.T) {
		cfg := loadCanonicalDefaultBackendConfig(t)
		cfg.Server.CORSAllowedOrigins = []string{"https://changed.example"}
		got := mustCanonicalJSON(t, captureBackendEffectiveConfig(t, cfg, 0))
		if bytes.Equal(got, want) {
			t.Fatal("mutated CORSAllowedOrigins matched golden; expected diff")
		}
	})

	t.Run("empty CORS origins fails startup validation", func(t *testing.T) {
		cfg := loadCanonicalDefaultBackendConfig(t)
		cfg.Server.CORSAllowedOrigins = nil
		if err := config.ValidateBackendConfig(cfg); err == nil {
			t.Fatal("ValidateBackendConfig succeeded with empty CORSAllowedOrigins; want startup error")
		} else if !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
			t.Fatalf("ValidateBackendConfig error = %q, want mention of CORS_ALLOWED_ORIGINS", err.Error())
		}
	})
}

func loadCanonicalDefaultBackendConfig(t *testing.T) *config.Config {
	t.Helper()
	// Clear server/CORS overrides so config.NewConfig exercises production defaults.
	for _, k := range []string{
		"APP_ENV",
		"SERVER_PORT",
		"CONNECT_PORT",
		"SERVER_READ_TIMEOUT",
		"SERVER_WRITE_TIMEOUT",
		"SERVER_IDLE_TIMEOUT",
		"CORS_ALLOWED_ORIGINS",
		"SOVEREIGN_URL",
		"SOVEREIGN_EVENT_TOKEN",
		"SOVEREIGN_EVENT_TOKEN_FILE",
		"SOVEREIGN_EVENT_AUTH",
		"HOST_RATE_LIMITER_REDIS_URL",
		"IMAGE_PROXY_SECRET",
		"IMAGE_PROXY_SECRET_FILE",
		"BACKEND_TOKEN_VERIFY_KEY_FILE",
		"RAG_API_AUTH",
		"RAG_API_TOKEN",
		"RAG_API_TOKEN_FILE",
		"MQHUB_ENABLED",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("IMAGE_PROXY_ENABLED", "false")

	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatalf("config.NewConfig() failed: %v", err)
	}

	// Write a temporary Ed25519 public key PEM so ValidateBackendConfig can run
	// all real startup checks including CORS_ALLOWED_ORIGINS validation.
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki})
	keyFile := filepath.Join(t.TempDir(), "verify.pub.pem")
	if err := os.WriteFile(keyFile, pubPEM, fsperm.SecretFile); err != nil {
		t.Fatalf("write verify key: %v", err)
	}
	t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", keyFile)

	cfg.Auth.BackendTokenPublicKey = pub
	cfg.Sovereign.URL = "http://knowledge-sovereign:9500"
	cfg.Rag.APIToken = "test-rag-api-token-minimum-24-characters"
	cfg.WebPush.PublicKey = "test-vapid-public-key"
	cfg.ReadingQueue.Cap = 7
	cfg.ReadingQueue.TTLHours = 48

	if err := config.ValidateBackendConfig(cfg); err != nil {
		t.Fatalf("ValidateBackendConfig(default) unexpected error: %v", err)
	}
	return cfg
}

func captureBackendEffectiveConfig(t *testing.T, cfg *config.Config, overrideBodyLimitBytes int64) effectiveConfigSnapshot {
	t.Helper()
	logger.InitLogger()
	log := slog.New(slog.DiscardHandler)

	limiter, err := connectmiddleware.NewUserQuotaLimiter(config.DefaultUserQuotaConfig(), time.Now, log)
	if err != nil {
		t.Fatalf("NewUserQuotaLimiter: %v", err)
	}
	container := &di.ApplicationComponents{
		CSRFTokenUsecase: csrf_token_usecase.NewCSRFTokenUsecase(goldenStubCSRFGateway{}),
		UserQuotaLimiter: limiter,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// 1. Capture Echo routes and middleware stack directly from real newRESTEcho + rest.RegisterRoutes.
	e := newRESTEcho(container, cfg, false, log)
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
	rest.RegisterRoutes(ctx, e, container, cfg)

	globalMws := middlewareNames(e.Middlewares())
	routes := make([]routeSnapshot, 0, len(captured))
	for _, r := range captured {
		stack := make([]string, 0, len(globalMws)+len(r.mws))
		stack = append(stack, globalMws...)
		stack = append(stack, r.mws...)
		routes = append(routes, routeSnapshot{
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

	// Capture OTel-enabled global middleware prefix from newRESTEcho(..., otelEnabled=true, ...).
	eOTel := newRESTEcho(container, cfg, true, log)
	otelMws := middlewareNames(eOTel.Middlewares())

	// 2. Runtime-probe effective BodyLimit exact byte boundary on a live wired Echo instance.
	eProbe := newRESTEcho(container, cfg, false, log)
	rest.RegisterRoutes(ctx, eProbe, container, cfg)
	if overrideBodyLimitBytes > 0 {
		// Used strictly by mutation negative test to prove runtime probe catches BodyLimit drift.
		eProbe.Use(echomiddleware.BodyLimit(overrideBodyLimitBytes))
	}
	eProbe.POST("/v1/_golden_body_probe", func(c *echo.Context) error {
		if _, err := io.ReadAll(c.Request().Body); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})
	bodyLimitBytes := probeExactBodyLimit(t, eProbe, "/v1/_golden_body_probe", 4<<20)

	// 3. Runtime-probe CORS preflight behavior and empty-origins fail-fast startup error.
	originsToProbe := append([]string(nil), cfg.Server.CORSAllowedOrigins...)
	originsToProbe = append(originsToProbe, "https://untrusted.example")
	preflightResults := make(map[string]string, len(originsToProbe))
	var secHeaders map[string]string
	for _, origin := range originsToProbe {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/v1/health", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		preflightResults[origin] = rec.Header().Get("Access-Control-Allow-Origin")
		if secHeaders == nil {
			secHeaders = map[string]string{
				"Content-Security-Policy": rec.Header().Get("Content-Security-Policy"),
				"Referrer-Policy":         rec.Header().Get("Referrer-Policy"),
				"X-Content-Type-Options":  rec.Header().Get("X-Content-Type-Options"),
				"X-Frame-Options":         rec.Header().Get("X-Frame-Options"),
				"X-XSS-Protection":        rec.Header().Get("X-XSS-Protection"),
			}
		}
	}

	emptyCfg := *cfg
	emptyCfg.Server.CORSAllowedOrigins = nil
	emptyCORSErr := ""
	if err := config.ValidateBackendConfig(&emptyCfg); err != nil {
		emptyCORSErr = err.Error()
	}

	// 4. Construct all 4 real HTTP servers via bootstrap constructors and capture timeouts.
	restSrv := bootstrap.NewRESTServer(fmt.Sprintf(":%d", cfg.Server.Port), e, cfg)
	connectSrv := bootstrap.NewConnectServer(fmt.Sprintf(":%d", cfg.Server.ConnectPort), http.NewServeMux())
	operatorSrv := bootstrap.NewServiceServer("127.0.0.1:9102", http.NewServeMux(), cfg)
	opsSrv := bootstrap.NewOpsServer(":9110", bootstrap.NewOpsHandler(serviceName, nil), cfg)

	servers := map[string]serverTimeout{
		"connect":  extractServerTimeout(connectSrv),
		"operator": extractServerTimeout(operatorSrv),
		"ops":      extractServerTimeout(opsSrv),
		"rest":     extractServerTimeout(restSrv),
	}

	originsCopy := append([]string(nil), cfg.Server.CORSAllowedOrigins...)
	return effectiveConfigSnapshot{
		Service:               serviceName,
		BodyLimitBytes:        bodyLimitBytes,
		GlobalMiddlewares:     globalMws,
		OTelGlobalMiddlewares: otelMws,
		Routes:                routes,
		SecurityHeaders:       secHeaders,
		Servers:               servers,
		CORS: corsSnapshot{
			AllowedOrigins:           originsCopy,
			EmptyOriginsStartupError: emptyCORSErr,
			PreflightResults:         preflightResults,
		},
	}
}

func probeExactBodyLimit(t *testing.T, h http.Handler, path string, maxSearch int64) int64 {
	t.Helper()
	payload := bytes.Repeat([]byte("a"), int(maxSearch)+1)
	accepted := func(n int64) bool {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(payload[:n]))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set("X-CSRF-Token", "golden-csrf")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code != http.StatusRequestEntityTooLarge
	}
	if !accepted(0) {
		t.Fatalf("body limit probe rejected 0-byte body on %s", path)
	}
	if accepted(maxSearch) {
		return 0
	}
	lo, hi := int64(0), maxSearch
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if accepted(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
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
	var protos []string
	if srv.Protocols != nil {
		if srv.Protocols.HTTP1() {
			protos = append(protos, "http1")
		}
		if srv.Protocols.HTTP2() {
			protos = append(protos, "http2")
		}
		if srv.Protocols.UnencryptedHTTP2() {
			protos = append(protos, "unencrypted_http2")
		}
	} else {
		protos = []string{"default"}
	}
	return serverTimeout{
		Addr:              srv.Addr,
		ReadHeaderTimeout: srv.ReadHeaderTimeout.String(),
		ReadTimeout:       srv.ReadTimeout.String(),
		WriteTimeout:      srv.WriteTimeout.String(),
		IdleTimeout:       srv.IdleTimeout.String(),
		MaxHeaderBytes:    srv.MaxHeaderBytes,
		Protocols:         protos,
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
