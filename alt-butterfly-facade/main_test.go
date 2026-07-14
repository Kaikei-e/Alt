package main

import (
	"net/http"
	"net/http/httptest"
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

	serverCfg := buildServerConfig(cfg, "http://alt-backend:9101", "", "", secret)

	assert.True(t, serverCfg.BFFConfig.EnableCache, "cfg.EnableCache must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableCircuitBreaker, "cfg.EnableCircuitBreaker must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableDedup, "cfg.EnableDedup must reach serverCfg.BFFConfig")
	assert.True(t, serverCfg.BFFConfig.EnableErrorNormalization, "cfg.EnableErrorNormalization must reach serverCfg.BFFConfig")
	assert.Equal(t, cfg.CacheMaxSize, serverCfg.BFFConfig.CacheMaxSize)
	assert.Equal(t, cfg.CacheDefaultTTL, serverCfg.BFFConfig.CacheDefaultTTL)
	assert.Equal(t, cfg.CBFailureThreshold, serverCfg.BFFConfig.CBFailureThreshold)
	assert.Equal(t, cfg.CBSuccessThreshold, serverCfg.BFFConfig.CBSuccessThreshold)
	assert.Equal(t, cfg.CBOpenTimeout, serverCfg.BFFConfig.CBOpenTimeout)
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

	serverCfg := buildServerConfig(cfg, "http://127.0.0.1:1", "", "", secret)
	handler := server.NewServerWithTransport(serverCfg, nil, http.DefaultTransport)

	req := httptest.NewRequest(http.MethodGet, "/v1/bff/stats", nil)
	req.Header.Set("X-Alt-Backend-Token", adminToken(t, secret, cfg.BackendTokenIssuer, cfg.BackendTokenAudience))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"cache"`, "expected BFFHandler cache stats; got %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"circuit_breaker"`, "expected BFFHandler circuit breaker stats; got %s", rec.Body.String())
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
