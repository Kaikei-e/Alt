package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/config"
	"alt/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

const testBackendTokenSecret = "test-backend-token-secret-minimum-32!"

// issueTestJWT creates a valid JWT for testing.
func issueTestJWT(t *testing.T, userID, email, role, sid string) string {
	t.Helper()
	now := time.Now()
	claims := BackendClaims{
		Email: email,
		Role:  role,
		Sid:   sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "auth-hub",
			Audience:  jwt.ClaimStrings{"alt-backend"},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testBackendTokenSecret))
	require.NoError(t, err)
	return signed
}

func testAuthConfig() *config.Config {
	return &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   testBackendTokenSecret,
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
}

func TestRequireAuth_SetsUserContext(t *testing.T) {
	e := echo.New()
	userID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	jwtToken := issueTestJWT(t, userID, "user@example.com", "admin", "session-token")
	req.Header.Set(backendTokenHeader, jwtToken)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.RequireAuth()(func(c echo.Context) error {
		called = true

		user, err := domain.GetUserFromContext(c.Request().Context())
		require.NoError(t, err)
		require.Equal(t, userID, user.UserID.String())
		require.Equal(t, userID, user.TenantID.String()) // Single-tenant
		require.Equal(t, "user@example.com", user.Email)
		require.Equal(t, domain.UserRoleAdmin, user.Role)
		require.Equal(t, "session-token", user.SessionID)

		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}

func TestRequireAuth_MissingJWT(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	// No JWT token
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	h := middleware.RequireAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestRequireAuth_InvalidJWT(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set(backendTokenHeader, "invalid-jwt-token")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	h := middleware.RequireAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestRequireAuth_SharedSecretOnlyFails(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "user@example.com")
	req.Header.Set("X-Alt-Shared-Secret", "any-secret")
	// No JWT token - shared secret alone should NOT work
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	h := middleware.RequireAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestOptionalAuth_AllowsUnauthenticatedRequests(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	// No auth headers at all
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err)
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}

// V-005: Test that OptionalAuth rejects requests with identity headers but no valid JWT
func TestOptionalAuth_RejectsUntrustedAuthHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	// Attacker sends auth headers but no valid JWT
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "attacker@evil.com")
	req.Header.Set(userRoleHeader, string(domain.UserRoleAdmin))
	// No JWT token - simulating direct attack
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should NOT have user context - treated as anonymous
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err, "user context should NOT be set when auth headers present without valid JWT")
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err, "request should continue as anonymous, not fail")
	require.True(t, called)
}

// V-005: Test that OptionalAuth accepts valid JWT
func TestOptionalAuth_AcceptsValidJWT(t *testing.T) {
	e := echo.New()
	userID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	jwtToken := issueTestJWT(t, userID, "user@example.com", "user", "session-123")
	req.Header.Set(backendTokenHeader, jwtToken)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should have user context
		user, err := domain.GetUserFromContext(c.Request().Context())
		require.NoError(t, err, "user context should be set with valid JWT")
		require.Equal(t, userID, user.UserID.String())
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}

// V-005: Test that OptionalAuth with invalid JWT is treated as anonymous
func TestOptionalAuth_InvalidJWTTreatedAsAnonymous(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	req.Header.Set(backendTokenHeader, "invalid-jwt-token")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should NOT have user context - treated as anonymous
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err, "user context should NOT be set with invalid JWT")
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err, "request should continue as anonymous")
	require.True(t, called)
}

// V-005: Test that requests without any headers are anonymous
func TestOptionalAuth_NoHeadersIsAnonymous(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	// No headers at all
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := testAuthConfig()
	middleware := NewAuthMiddleware(nil, cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err, "user context should not be set for anonymous request")
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}
