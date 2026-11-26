package bootstrap

import (
	"bytes"
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

	"pre-processor/internal/fsperm"
	"pre-processor/internal/httpserver"
	"pre-processor/tlsutil"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

type preProcessorEffectiveConfig struct {
	BodyLimitBytes        int64                    `json:"body_limit_bytes"`
	CORSAllowedOrigins    []string                 `json:"cors_allowed_origins"`
	GlobalMiddlewares     []string                 `json:"global_middlewares"`
	OTelGlobalMiddlewares []string                 `json:"otel_global_middlewares"`
	Routes                []routeSnapshot          `json:"routes"`
	Servers               map[string]serverTimeout `json:"servers"`
	Service               string                   `json:"service"`
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

const (
	goldenDirPath  = "testdata"
	goldenFilePath = "testdata/effective_config.golden.json"
)

func TestEffectiveConfigGolden(t *testing.T) {
	snap := capturePreProcessorEffectiveConfig(t, newHTTPListenerServer, 0)
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

	t.Run("C-15 mutation shifting BodyLimit from 1000000 to 1<<20 (1048576) produces golden diff", func(t *testing.T) {
		const mutatedMiBLimitBytes int64 = 1 << 20
		snap := capturePreProcessorEffectiveConfig(t, newHTTPListenerServer, mutatedMiBLimitBytes)
		if snap.BodyLimitBytes != mutatedMiBLimitBytes {
			t.Fatalf("mutated probe body_limit_bytes = %d, want %d", snap.BodyLimitBytes, mutatedMiBLimitBytes)
		}
		got := mustCanonicalJSON(t, snap)
		if bytes.Equal(got, want) {
			t.Fatal("C-15 BodyLimit 1<<20 mutation matched golden; expected diff")
		}
	})

	t.Run("C-11 mutation adding IdleTimeout 60s to HTTP listener produces golden diff", func(t *testing.T) {
		mutatedHTTP := func(addr string, handler http.Handler) *http.Server {
			return httpserver.New(httpserver.Config{
				Addr:              addr,
				Handler:           handler,
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       60 * time.Second,
			})
		}
		got := mustCanonicalJSON(t, capturePreProcessorEffectiveConfig(t, mutatedHTTP, 0))
		if bytes.Equal(got, want) {
			t.Fatal("C-11 IdleTimeout mutation matched golden; expected diff")
		}
	})
}

func capturePreProcessorEffectiveConfig(
	t *testing.T,
	buildHTTP func(string, http.Handler) *http.Server,
	mutatedLimitBytes int64,
) preProcessorEffectiveConfig {
	t.Helper()
	t.Setenv("MTLS_ALLOWED_PEERS", "")
	deps := newTestHTTPServer(t, "golden-test")

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
	configureHTTPServer(e, deps, false, "")

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

	eOTel := echo.New()
	configureHTTPServer(eOTel, deps, true, "pre-processor")
	otelMws := middlewareNames(eOTel.Middlewares())

	// Runtime probe of effective BodyLimit exact byte boundary on POST /api/v1/summarize.
	var probeHandler http.Handler = e
	if mutatedLimitBytes > 0 {
		// Replicate C-15 bug (1 << 20 instead of 1_000_000) to prove runtime probe catches it.
		eMut := echo.New()
		api := eMut.Group("/api/v1")
		api.Use(middleware.BodyLimit(mutatedLimitBytes))
		api.POST("/summarize", func(c *echo.Context) error {
			return c.NoContent(http.StatusBadRequest)
		})
		probeHandler = eMut
	}
	bodyLimitBytes := probeExactBodyLimit(t, probeHandler, "/api/v1/summarize", 2<<20)

	// Runtime probe confirming CORS is absent on pre-processor.
	optReq := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/summarize", nil)
	optReq.Header.Set("Origin", "https://evil.example")
	optReq.Header.Set("Access-Control-Request-Method", http.MethodPost)
	optRec := httptest.NewRecorder()
	e.ServeHTTP(optRec, optReq)
	corsOrigins := []string{}
	if allow := optRec.Header().Get("Access-Control-Allow-Origin"); allow != "" {
		corsOrigins = append(corsOrigins, allow)
	}

	httpSrv := buildHTTP(":"+httpPort, e)
	connectSrv := newConnectListenerServer(":"+connectPort, http.NewServeMux())
	mtlsSrv := tlsutil.NewMTLSHTTPServer(":9443", nil, NewMTLSMuxHandler(e, http.NewServeMux(), nil, slog.New(slog.DiscardHandler)))

	return preProcessorEffectiveConfig{
		Service:               "pre-processor",
		BodyLimitBytes:        bodyLimitBytes,
		CORSAllowedOrigins:    corsOrigins,
		GlobalMiddlewares:     globalMws,
		OTelGlobalMiddlewares: otelMws,
		Routes:                routes,
		Servers: map[string]serverTimeout{
			"connect": extractServerTimeout(connectSrv),
			"http":    extractServerTimeout(httpSrv),
			"mtls":    extractServerTimeout(mtlsSrv),
		},
	}
}

func probeExactBodyLimit(t *testing.T, h http.Handler, path string, maxSearch int64) int64 {
	t.Helper()
	payload := bytes.Repeat([]byte("a"), int(maxSearch)+1)
	accepted := func(n int64) bool {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(payload[:n]))
		req.Header.Set("Content-Type", "application/octet-stream")
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
