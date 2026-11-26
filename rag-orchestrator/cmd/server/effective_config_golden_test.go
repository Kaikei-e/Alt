package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
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

	"rag-orchestrator/internal/di"
	"rag-orchestrator/internal/fsperm"
	"rag-orchestrator/internal/httpserver"
	"rag-orchestrator/internal/infra/config"
	peermw "rag-orchestrator/internal/middleware"

	"github.com/labstack/echo/v5"
)

var updateGolden = flag.Bool("update", false, "update effective config golden file")

type ragEffectiveConfig struct {
	BodyLimitBytes     int64                    `json:"body_limit_bytes"`
	CORSAllowedOrigins []string                 `json:"cors_allowed_origins"`
	GlobalMiddlewares  []string                 `json:"global_middlewares"`
	Routes             []ragRouteSnapshot       `json:"routes"`
	Servers            map[string]serverTimeout `json:"servers"`
	Service            string                   `json:"service"`
}

type ragRouteSnapshot struct {
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
	TLSClientAuth     string   `json:"tls_client_auth,omitempty"`
	TLSMinVersion     string   `json:"tls_min_version,omitempty"`
	WriteTimeout      string   `json:"write_timeout"`
}

const (
	goldenDirPath  = "testdata"
	goldenFilePath = "testdata/effective_config.golden.json"
)

func TestEffectiveConfigGolden(t *testing.T) {
	snap := captureRagEffectiveConfig(t, newPlaintextConnectServer)
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

	t.Run("C-11 mutation adding IdleTimeout 60s to plaintext Connect server produces golden diff", func(t *testing.T) {
		mutatedConnect := func(addr string, handler http.Handler) *http.Server {
			return httpserver.New(httpserver.Config{
				Addr:              addr,
				Handler:           handler,
				Protocols:         cleartextHTTP2(),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       60 * time.Second,
			})
		}
		got := mustCanonicalJSON(t, captureRagEffectiveConfig(t, mutatedConnect))
		if bytes.Equal(got, want) {
			t.Fatal("C-11 IdleTimeout mutation matched golden; expected diff")
		}
	})

	t.Run("middleware order mutation produces golden diff", func(t *testing.T) {
		snap := captureRagEffectiveConfig(t, newPlaintextConnectServer)
		if len(snap.GlobalMiddlewares) < 4 {
			t.Fatalf("expected at least 4 global middlewares, got %d", len(snap.GlobalMiddlewares))
		}
		snap.GlobalMiddlewares[2], snap.GlobalMiddlewares[3] = snap.GlobalMiddlewares[3], snap.GlobalMiddlewares[2]
		got := mustCanonicalJSON(t, snap)
		if bytes.Equal(got, want) {
			t.Fatal("middleware reorder mutation matched golden; expected diff")
		}
	})
}

func captureRagEffectiveConfig(t *testing.T, buildPlaintextConnect func(string, http.Handler) *http.Server) ragEffectiveConfig {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	peerMW := peermw.NewPeerIdentityMiddleware([]string{"alt-backend", "alt-harvester"}, log)
	apiAuthMW := peermw.NewAPIAuthMiddleware("test-rag-api-token-minimum-24-characters", true, log)

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
	configureRESTEcho(e, &di.ApplicationComponents{}, &config.Config{}, peerMW, apiAuthMW, nil, log)

	globalMws := middlewareNames(e.Middlewares())
	routes := make([]ragRouteSnapshot, 0, len(captured))
	for _, r := range captured {
		stack := make([]string, 0, len(globalMws)+len(r.mws))
		stack = append(stack, globalMws...)
		stack = append(stack, r.mws...)
		routes = append(routes, ragRouteSnapshot{
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

	// Runtime probe confirming CORS is absent on rag-orchestrator.
	optReq := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/healthz", nil)
	optReq.Header.Set("Origin", "https://evil.example")
	optReq.Header.Set("Access-Control-Request-Method", http.MethodGet)
	optRec := httptest.NewRecorder()
	e.ServeHTTP(optRec, optReq)
	corsOrigins := []string{}
	if allow := optRec.Header().Get("Access-Control-Allow-Origin"); allow != "" {
		corsOrigins = append(corsOrigins, allow)
	}

	// Build all real HTTP/Connect/TLS/health servers.
	echoSrv := newEchoHTTPServer(":9010", e)
	healthSrv := newHealthServer(":9012", nil, log)
	plaintextConnectSrv := buildPlaintextConnect(":9011", http.NewServeMux())

	certFile, keyFile, caFile := writeEphemeralTestCertBundle(t)
	tlsConnectSrv, err := newProductionTLSServer(":9011", http.NewServeMux(), certFile, keyFile, caFile)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}

	return ragEffectiveConfig{
		Service:            "rag-orchestrator",
		BodyLimitBytes:     0,
		CORSAllowedOrigins: corsOrigins,
		GlobalMiddlewares:  globalMws,
		Routes:             routes,
		Servers: map[string]serverTimeout{
			"connect_plaintext": extractServerTimeout(plaintextConnectSrv),
			"connect_tls":       extractServerTimeout(tlsConnectSrv),
			"echo":              extractServerTimeout(echoSrv),
			"health":            extractServerTimeout(healthSrv),
		},
	}
}

func writeEphemeralTestCertBundle(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "rag-orchestrator"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create self-signed cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")
	caFile = filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(certFile, certPEM, fsperm.GeneratedFile); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, fsperm.SecretFile); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(caFile, certPEM, fsperm.GeneratedFile); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	return certFile, keyFile, caFile
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
	st := serverTimeout{
		Addr:              srv.Addr,
		ReadHeaderTimeout: srv.ReadHeaderTimeout.String(),
		ReadTimeout:       srv.ReadTimeout.String(),
		WriteTimeout:      srv.WriteTimeout.String(),
		IdleTimeout:       srv.IdleTimeout.String(),
		MaxHeaderBytes:    srv.MaxHeaderBytes,
		Protocols:         protos,
	}
	if srv.TLSConfig != nil {
		st.TLSMinVersion = tlsVersionLabel(srv.TLSConfig.MinVersion)
		st.TLSClientAuth = srv.TLSConfig.ClientAuth.String()
	}
	return st
}

func tlsVersionLabel(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	default:
		return fmt.Sprintf("0x%04x", v)
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
