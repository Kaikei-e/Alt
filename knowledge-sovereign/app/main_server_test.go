package main

import (
	"bytes"
	"context"
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

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/config"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
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

type recordingAppendHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	called bool
}

func (h *recordingAppendHandler) AppendKnowledgeEvent(
	_ context.Context,
	_ *connect.Request[sovereignv1.AppendKnowledgeEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeEventResponse], error) {
	h.called = true
	return connect.NewResponse(&sovereignv1.AppendKnowledgeEventResponse{EventSeq: 1}), nil
}

func TestBuildRPCMux_RejectsOversizedRequestWithResourceExhausted(t *testing.T) {
	const token = "test-event-token"
	cfg := &config.Config{
		EventToken:       token,
		EventAuthEnabled: true,
	}
	stub := &recordingAppendHandler{}
	srv := httptest.NewServer(buildRPCMux(stub, cfg))
	t.Cleanup(srv.Close)

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: &sovereignv1.KnowledgeEvent{
			Payload: bytes.Repeat([]byte("a"), 2<<20),
		},
	})
	req.Header().Set("Authorization", "Bearer "+token)

	_, err := client.AppendKnowledgeEvent(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.False(t, stub.called, "oversized request must be rejected before decoding or invoking the handler")
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
