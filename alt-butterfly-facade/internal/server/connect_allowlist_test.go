package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllowConnectPath_ReadingServiceRecordReadingEvents(t *testing.T) {
	const path = "/alt.reading.v1.ReadingService/RecordReadingEvents"
	assert.True(t, allowConnectPath(path), "path %q", path)
}

func TestServer_ForwardsReadingServiceRecordReadingEvents(t *testing.T) {
	var seenPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer backend.Close()

	cfg := Config{
		BackendURL:       backend.URL,
		TokenVerifyKey:   testPublicKey,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}
	h := NewServerWithTransport(cfg, nil, http.DefaultTransport)

	const path = "/alt.reading.v1.ReadingService/RecordReadingEvents"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, testPrivateKey))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, path, seenPath)
}
