package middleware_test

import (
	"alt/config"
	"alt/middleware"
	"alt/shared/domain/authcontext"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reproTestSecret = "test-backend-token-secret-minimum-32!"

func issueReproTestJWT(t *testing.T, userID, email string) string {
	t.Helper()
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":       userID,
		"email":     email,
		"role":      "user",
		"sid":       "test-session",
		"tenant_id": userID,
		"iss":       "auth-hub",
		"aud":       []string{"alt-backend"},
		"iat":       now.Unix(),
		"exp":       now.Add(5 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(reproTestSecret))
	require.NoError(t, err)
	return signed
}

func TestAuthMiddleware_Bypass(t *testing.T) {
	// Setup
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.AuthConfig{
		BackendTokenSecret:   reproTestSecret,
		BackendTokenIssuer:   "auth-hub",
		BackendTokenAudience: "alt-backend",
	}
	authMiddleware := middleware.NewAuthMiddleware(logger, cfg)

	// Define a protected route
	e.GET("/protected", func(c echo.Context) error {
		return c.String(http.StatusOK, "Secret Data")
	}, authMiddleware.RequireAuth())

	// Case 1: No headers - should fail
	t.Run("Missing Headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	// Case 2: Arbitrary headers WITHOUT JWT - should fail
	t.Run("Arbitrary Headers Without JWT", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("X-Alt-User-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Tenant-Id", "123e4567-e89b-12d3-a456-426614174000")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Arbitrary headers without JWT rejected")
	})

	// Case 3: Shared secret only (no JWT) - should fail
	t.Run("Shared Secret Only Without JWT", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("X-Alt-User-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Tenant-Id", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-Alt-Shared-Secret", "any-secret-value")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Shared secret alone no longer grants access")
	})

	// Case 4: Valid JWT - should succeed
	t.Run("Valid JWT Grants Access", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		jwtToken := issueReproTestJWT(t, "123e4567-e89b-12d3-a456-426614174000", "user@example.com")
		req.Header.Set("X-Alt-Backend-Token", jwtToken)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "Valid JWT allowed access")
	})
}

func TestAuthMiddleware_PropagatesJWTToAuthContext(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.AuthConfig{
		BackendTokenSecret:   reproTestSecret,
		BackendTokenIssuer:   "auth-hub",
		BackendTokenAudience: "alt-backend",
	}
	authMiddleware := middleware.NewAuthMiddleware(logger, cfg)

	var gotRequireToken, gotOptionalToken string
	e.GET("/require", func(c echo.Context) error {
		tok, _ := authcontext.JWTFromContext(c.Request().Context())
		gotRequireToken = tok
		return c.NoContent(http.StatusOK)
	}, authMiddleware.RequireAuth())

	e.GET("/optional", func(c echo.Context) error {
		tok, _ := authcontext.JWTFromContext(c.Request().Context())
		gotOptionalToken = tok
		return c.NoContent(http.StatusOK)
	}, authMiddleware.OptionalAuth())

	jwtToken := issueReproTestJWT(t, "123e4567-e89b-12d3-a456-426614174000", "user@example.com")

	req1 := httptest.NewRequest(http.MethodGet, "/require", nil)
	req1.Header.Set("X-Alt-Backend-Token", jwtToken)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, jwtToken, gotRequireToken)

	req2 := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req2.Header.Set("X-Alt-Backend-Token", jwtToken)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, jwtToken, gotOptionalToken)
}
