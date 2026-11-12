package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/config"
)

func TestNewHTTPServer(t *testing.T) {
	mux := http.NewServeMux()
	srv := newHTTPServer(":9500", mux, 30*time.Second)

	if srv.Addr != ":9500" {
		t.Errorf("Addr = %s, want :9500", srv.Addr)
	}
	if srv.Handler != mux {
		t.Error("Handler not properly set")
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want 30s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
}

func TestBuildRPCMux_HealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		EventToken:       "test-event-token",
		EventAuthEnabled: false,
	}
	mux := buildRPCMux(testSovereignHandler{}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for health endpoint, got %d", rec.Code)
	}
}

func TestLogUserTokenVerifierStatus(t *testing.T) {
	for _, mode := range []string{"introspection", "local", "none"} {
		t.Run(mode, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			logUserTokenVerifierStatus(&config.Config{UserTokenVerifierMode: mode})

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			require.Len(t, lines, 1)

			var rec map[string]any
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &rec))
			assert.Equal(t, "INFO", rec["level"])
			assert.Equal(t, "user_token_verifier", rec["msg"])
			assert.Equal(t, mode, rec["mode"])
		})
	}
}

func TestStartServers_LogsUserTokenVerifierOnce(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main_server.go", nil, 0)
	require.NoError(t, err)

	var startFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "startServers" && fn.Recv == nil {
			startFn = fn
			break
		}
	}
	require.NotNil(t, startFn)

	calls := 0
	ast.Inspect(startFn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "logUserTokenVerifierStatus" {
				calls++
			}
		}
		return true
	})
	assert.Equal(t, 1, calls, "startServers must call logUserTokenVerifierStatus exactly once")
}
