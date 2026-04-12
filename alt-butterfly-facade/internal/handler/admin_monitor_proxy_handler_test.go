package handler

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"alt-butterfly-facade/internal/client"
)

func newMonitorProxy(t *testing.T, backendURL string) *AdminMonitorProxyHandler {
	t.Helper()
	backendClient := client.NewBackendClientWithTransport(
		backendURL,
		30*time.Second,
		30*time.Minute,
		http.DefaultTransport,
	)
	return NewAdminMonitorProxyHandler(
		backendClient,
		[]byte("test-secret"),
		"auth-hub",
		"alt-backend",
		"service-secret",
		nil,
	)
}

func TestAdminMonitorProxy_RequiresAdminRole(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("backend must not be called for non-admin")
	}))
	defer backend.Close()
	h := newMonitorProxy(t, backend.URL)

	req := httptest.NewRequest(http.MethodPost, "/alt.admin_monitor.v1.AdminMonitorService/Snapshot", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createRoleToken(t, []byte("test-secret"), "user"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAdminMonitorProxy_RequiresAuth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("backend must not be called without auth")
	}))
	defer backend.Close()
	h := newMonitorProxy(t, backend.URL)

	req := httptest.NewRequest(http.MethodPost, "/alt.admin_monitor.v1.AdminMonitorService/Snapshot", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAdminMonitorProxy_ForwardsUnaryWithServiceToken(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.admin_monitor.v1.AdminMonitorService/Snapshot", r.URL.Path)
		assert.Equal(t, "service-secret", r.Header.Get("X-Service-Token"))
		assert.Empty(t, r.Header.Get("X-Alt-Backend-Token"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"time":"2026-04-12T00:00:00Z","metrics":[]}`))
	}))
	defer backend.Close()
	h := newMonitorProxy(t, backend.URL)

	req := httptest.NewRequest(http.MethodPost, "/alt.admin_monitor.v1.AdminMonitorService/Snapshot", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createRoleToken(t, []byte("test-secret"), "admin"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"metrics"`)
}

func TestAdminMonitorProxy_StreamingFlushesChunks(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.admin_monitor.v1.AdminMonitorService/Watch", r.URL.Path)
		assert.Equal(t, "service-secret", r.Header.Get("X-Service-Token"))
		w.Header().Set("Content-Type", "application/connect+json")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			if _, err := w.Write([]byte("frame-")); err != nil {
				return
			}
			if _, err := w.Write([]byte{byte('0' + i), '\n'}); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	defer backend.Close()
	h := newMonitorProxy(t, backend.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use a real httptest server so we have a live client pipe that streams,
	// since httptest.ResponseRecorder does not propagate Flush semantics.
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := strings.NewReader(`{"keys":["availability_services"]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/alt.admin_monitor.v1.AdminMonitorService/Watch", body)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createRoleToken(t, []byte("test-secret"), "admin"))

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Backbone headers must survive the hop.
	assert.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	// Receive at least 2 frames before deadline to prove streaming (not buffered).
	r := bufio.NewReader(resp.Body)
	got := 0
	for got < 2 {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, "frame-") {
			got++
		}
		if err == io.EOF {
			break
		}
	}
	assert.GreaterOrEqual(t, got, 2, "expected at least 2 streamed frames")
}
