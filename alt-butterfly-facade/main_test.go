package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/config"
	"alt-butterfly-facade/internal/server"
)

// TestBuildServerConfig_WiresBFFConfigFromAppConfig locks down the wiring
// bug in main.go: serverCfg := server.Config{...} never sets BFFConfig, so
// cfg.EnableCache / EnableCircuitBreaker / EnableDedup /
// EnableErrorNormalization (all true by default, config/config.go:84-97)
// never reach server.NewServerWithTransports.
func TestBuildServerConfig_WiresBFFConfigFromAppConfig(t *testing.T) {
	cfg := config.NewConfig()
	secret := []byte("this-is-a-valid-backend-token-secret-32-chars-long")

	serverCfg := buildServerConfig(cfg, "http://alt-backend:9101", "http://alt-backend:9102", "", "", secret, "")

	assert.True(t, serverCfg.BFFConfig.EnableCache, "cfg.EnableCache must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableCircuitBreaker, "cfg.EnableCircuitBreaker must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableDedup, "cfg.EnableDedup must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableErrorNormalization, "cfg.EnableErrorNormalization must reach serverCfg.BFFConfig")
	assert.Equal(t, cfg.CacheMaxSize, serverCfg.BFFConfig.CacheMaxSize)
	assert.Equal(t, cfg.CacheDefaultTTL, serverCfg.BFFConfig.CacheDefaultTTL)
	assert.Equal(t, cfg.CBFailureThreshold, serverCfg.BFFConfig.CBFailureThreshold)
	assert.Equal(t, cfg.CBSuccessThreshold, serverCfg.BFFConfig.CBSuccessThreshold)
	assert.Equal(t, cfg.CBOpenTimeout, serverCfg.BFFConfig.CBOpenTimeout)
	assert.Equal(t, cfg.CBExternalContentFailureThreshold, serverCfg.BFFConfig.CBExternalContentFailureThreshold)
	assert.Equal(t, cfg.CBExternalContentOpenTimeout, serverCfg.BFFConfig.CBExternalContentOpenTimeout)
	assert.Equal(t, cfg.DedupWindow, serverCfg.BFFConfig.DedupWindow)
}

// TestBuildServerConfig_ResultingServer_UsesBFFHandler proves the wiring gap
// end-to-end: a server built from buildServerConfig's output must serve
// /v1/bff/stats with populated cache/circuit_breaker stats (proof that
// internal/server/server.go's feature switch picked the BFFHandler branch),
// not the empty `{}` it returns when bffHandler stays nil (legacy
// ProxyHandler branch, the current behavior with BFFConfig unset).
func TestBuildServerConfig_ResultingServer_UsesBFFHandler(t *testing.T) {
	cfg := config.NewConfig()
	secret := []byte("this-is-a-valid-backend-token-secret-32-chars-long")

	serverCfg := buildServerConfig(cfg, "http://127.0.0.1:1", "http://127.0.0.1:1", "", "", secret, "")
	handler := server.NewServerWithTransport(serverCfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(http.MethodGet, "/v1/bff/stats", nil)
	req.Header.Set("X-Alt-Backend-Token", adminToken(t, secret, cfg.BackendTokenIssuer, cfg.BackendTokenAudience))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"cache"`, "expected BFFHandler cache stats; got %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"circuit_breaker"`, "expected BFFHandler circuit breaker stats; got %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"external_content"`, "every dependency class must be observable on /stats; got %s", rec.Body.String())
}

// adminToken creates a valid admin-role JWT for the /v1/bff/stats auth check.
func adminToken(t *testing.T, secret []byte, issuer, audience string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub":  uuid.New().String(),
		"role": "admin",
		"iss":  issuer,
		"aud":  []string{audience},
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	require.NoError(t, err)
	return signed
}

// TestLogBFFFeatureWiring_TTSProxy pins requirement 4(e):
// Startup log bff.tts_proxy.wiring is emitted with enabled + reason (+ url when enabled).
func TestLogBFFFeatureWiring_TTSProxy(t *testing.T) {
	t.Run("enabled logs url and reason", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		slog.SetDefault(logger)

		cfg := config.NewConfig()
		cfg.TTSProxy = "enabled"
		cfg.TTSConnectURL = "http://tts-speaker:9700"

		logBFFFeatureWiring(context.Background(), cfg)

		logOutput := buf.String()
		assert.Contains(t, logOutput, `"msg":"bff.tts_proxy.wiring"`, "log line bff.tts_proxy.wiring must be emitted")
		assert.Contains(t, logOutput, `"enabled":true`)
		assert.Contains(t, logOutput, `"url":"http://tts-speaker:9700"`)
		assert.Contains(t, logOutput, `"reason":`)
	})

	t.Run("disabled logs false and reason", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		slog.SetDefault(logger)

		cfg := config.NewConfig()
		cfg.TTSProxy = "disabled"
		cfg.TTSConnectURL = ""

		logBFFFeatureWiring(context.Background(), cfg)

		logOutput := buf.String()
		assert.Contains(t, logOutput, `"msg":"bff.tts_proxy.wiring"`, "log line bff.tts_proxy.wiring must be emitted")
		assert.Contains(t, logOutput, `"enabled":false`)
		assert.Contains(t, logOutput, `"reason":`)
	})
}

// TestResolveTTSURL_MTLSOverride pins requirement 1 (and 4(d)):
// When MTLS_ENFORCE=true, TTS_CONNECT_MTLS_URL overrides TTS_CONNECT_URL (same shape as Acolyte).
func TestResolveTTSURL_MTLSOverride(t *testing.T) {
	os.Clearenv()
	os.Setenv("MTLS_ENFORCE", "true")
	os.Setenv("TTS_CONNECT_URL", "http://tts-speaker:9700")
	os.Setenv("TTS_CONNECT_MTLS_URL", "https://tts-speaker:9443")
	defer os.Clearenv()

	cfg := config.NewConfig()
	cfg.TTSProxy = "enabled"
	cfg.TTSConnectURL = "http://tts-speaker:9700"
	resolved := resolveTTSURL(cfg)
	assert.Equal(t, "https://tts-speaker:9443", resolved, "TTS_CONNECT_MTLS_URL must override TTS_CONNECT_URL when MTLS_ENFORCE=true")
}
