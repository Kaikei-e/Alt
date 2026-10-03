package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auth-hub/internal/domain"
	"auth-hub/internal/infrastructure/cache"
	"auth-hub/internal/infrastructure/token"
	"auth-hub/internal/usecase"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const a01TestJWTSecret = "a01-handler-jwt-secret-at-least-32-chars-long"

// TestA01_Handler_InvalidateThenSession exercises the production HTTP path:
//
//  1. GET /session → real JWT issued, cache populated.
//  2. POST /session/invalidate → cache purged via handler.
//  3. GET /session → must fail (simulated Kratos revoked).
//
// The JWT is minted by the real token.JWTIssuer, not a mock.
func TestA01_Handler_InvalidateThenSession(t *testing.T) {
	e := echo.New()

	// Build real infrastructure components.
	sessionCache := cache.NewSessionCache(60 * time.Second)
	realIssuer := token.NewJWTIssuer(token.JWTConfig{
		Secret:   a01TestJWTSecret,
		Issuer:   "auth-hub",
		Audience: "alt-backend",
		TTL:      5 * time.Minute,
	})

	// A validator that can be revoked mid-test.
	rawCookie := "handler-test-cookie-a01"
	fullCookie := "ory_kratos_session=" + rawCookie
	validator := &revocableValidatorForHandler{
		identities: map[string]*domain.Identity{
			fullCookie: {
				UserID:    "user-handler-a01",
				TenantID:  "user-handler-a01",
				Email:     "handler@example.com",
				Role:      "user",
				SessionID: "kratos-sess-handler-a01",
				CreatedAt: time.Now(),
			},
		},
	}

	getSessionUC := usecase.NewGetSession(validator, sessionCache, realIssuer, slog.Default())
	invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
	sessionHandler := NewSessionHandler(getSessionUC, invalidateUC)

	// Step 1: GET /session — must succeed, returning a real JWT.
	t.Run("step1_session_succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
		req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: rawCookie})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := sessionHandler.Handle(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify real JWT in response header.
		backendToken := rec.Header().Get("X-Alt-Backend-Token")
		require.NotEmpty(t, backendToken, "must return a backend token")

		// Parse and validate the JWT is real and well-formed.
		parsed, err := jwtlib.Parse(backendToken, func(t *jwtlib.Token) (interface{}, error) {
			return []byte(a01TestJWTSecret), nil
		})
		require.NoError(t, err)
		require.True(t, parsed.Valid)

		claims, ok := parsed.Claims.(jwtlib.MapClaims)
		require.True(t, ok)
		assert.Equal(t, "user-handler-a01", claims["sub"])
		assert.Equal(t, "kratos-sess-handler-a01", claims["sid"])

		// Verify JSON body
		var resp map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["ok"].(bool))
	})

	// Step 2: Revoke in Kratos, then POST /session/invalidate.
	validator.revoke(fullCookie)

	t.Run("step2_invalidate_succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
		req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: rawCookie})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := sessionHandler.HandleInvalidate(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]bool
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["ok"])
	})

	// Step 3: GET /session — must fail because cache was purged
	// and Kratos reports the session revoked.
	t.Run("step3_session_rejected_after_invalidation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/session", nil)
		req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: rawCookie})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := sessionHandler.Handle(c)
		require.Error(t, err, "A01: revoked session must fail at handler level")

		// Echo returns an HTTPError for 401.
		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, he.Code)
	})
}

// TestA01_Handler_InvalidateNon2xxHandling verifies that a non-2xx status
// from auth-hub's invalidation response is handled gracefully by the caller.
// This tests the handler's own error paths — an empty body, invalid cookie, etc.
func TestA01_Handler_InvalidateNon2xxHandling(t *testing.T) {
	e := echo.New()

	sessionCache := cache.NewSessionCache(60 * time.Second)
	invalidateUC := usecase.NewInvalidateSession(sessionCache, slog.Default())
	h := NewSessionHandler(nil, invalidateUC)

	t.Run("missing_cookie_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidate(c)
		require.Error(t, err)

		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, he.Code)
	})

	t.Run("empty_cookie_value_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/session/invalidate", nil)
		req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: ""})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.HandleInvalidate(c)
		require.Error(t, err)

		he, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, he.Code)
	})
}

// revocableValidatorForHandler is the handler-test analogue of
// mockRevocableValidator in the usecase tests.
type revocableValidatorForHandler struct {
	identities map[string]*domain.Identity
	revoked    map[string]bool
}

func (v *revocableValidatorForHandler) ValidateSession(_ context.Context, cookie string) (*domain.Identity, error) {
	if v.revoked != nil && v.revoked[cookie] {
		return nil, domain.ErrAuthFailed
	}
	id, ok := v.identities[cookie]
	if !ok {
		return nil, domain.ErrAuthFailed
	}
	return id, nil
}

func (v *revocableValidatorForHandler) revoke(fullCookie string) {
	if v.revoked == nil {
		v.revoked = make(map[string]bool)
	}
	v.revoked[fullCookie] = true
}
