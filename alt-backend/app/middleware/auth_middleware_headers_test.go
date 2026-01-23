package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/config"
	"alt/domain"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestRequireAuth_SetsUserContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "user@example.com")
	req.Header.Set(userRoleHeader, string(domain.UserRoleAdmin))
	req.Header.Set(sessionIDHeader, "session-token")
	req.Header.Set(sharedSecretHeader, "test-secret")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	called := false
	h := middleware.RequireAuth()(func(c echo.Context) error {
		called = true

		user, err := domain.GetUserFromContext(c.Request().Context())
		require.NoError(t, err)
		require.Equal(t, userID, user.UserID.String())
		require.Equal(t, tenantID, user.TenantID.String())
		require.Equal(t, "user@example.com", user.Email)
		require.Equal(t, domain.UserRoleAdmin, user.Role)
		require.Equal(t, "session-token", user.SessionID)
		require.WithinDuration(t, time.Now(), user.LoginAt, time.Second)
		require.WithinDuration(t, time.Now().Add(24*time.Hour), user.ExpiresAt, 2*time.Second)

		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}

func TestRequireAuth_MissingHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set(sharedSecretHeader, "test-secret")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	h := middleware.RequireAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusUnauthorized, httpErr.Code)
}

func TestRequireAuth_InvalidUUID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set(userIDHeader, "not-a-uuid")
	req.Header.Set(tenantIDHeader, uuid.New().String())
	req.Header.Set(sharedSecretHeader, "test-secret")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	h := middleware.RequireAuth()(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestOptionalAuth_AllowsUnauthenticatedRequests(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	req.Header.Set(sharedSecretHeader, "test-secret")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
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

// V-005: Test that OptionalAuth rejects untrusted auth headers
func TestOptionalAuth_RejectsUntrustedAuthHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	// Attacker sends auth headers but no valid shared secret
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "attacker@evil.com")
	req.Header.Set(userRoleHeader, string(domain.UserRoleAdmin))
	// No shared secret header - simulating direct attack
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should NOT have user context - treated as anonymous
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err, "user context should NOT be set when auth headers present without valid secret")
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err, "request should continue as anonymous, not fail")
	require.True(t, called)
}

// V-005: Test that OptionalAuth accepts valid secret with headers
func TestOptionalAuth_AcceptsValidSecretWithHeaders(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "user@example.com")
	req.Header.Set(userRoleHeader, string(domain.UserRoleUser))
	req.Header.Set(sharedSecretHeader, "test-secret") // Valid secret
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should have user context
		user, err := domain.GetUserFromContext(c.Request().Context())
		require.NoError(t, err, "user context should be set with valid secret")
		require.Equal(t, userID, user.UserID.String())
		require.Equal(t, tenantID, user.TenantID.String())
		return c.NoContent(http.StatusOK)
	})

	err := h(c)
	require.NoError(t, err)
	require.True(t, called)
}

// V-005: Test that OptionalAuth with invalid secret but auth headers is treated as anonymous
func TestOptionalAuth_InvalidSecretWithHeadersTreatedAsAnonymous(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/articles", nil)
	userID := uuid.New().String()
	tenantID := uuid.New().String()
	req.Header.Set(userIDHeader, userID)
	req.Header.Set(tenantIDHeader, tenantID)
	req.Header.Set(userEmailHeader, "attacker@evil.com")
	req.Header.Set(sharedSecretHeader, "wrong-secret") // Wrong secret
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
	called := false
	h := middleware.OptionalAuth()(func(c echo.Context) error {
		called = true
		// Should NOT have user context - treated as anonymous
		_, err := domain.GetUserFromContext(c.Request().Context())
		require.Error(t, err, "user context should NOT be set with invalid secret")
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

	cfg := &config.Config{
		Auth: config.AuthConfig{
			BackendTokenSecret:   "",
			BackendTokenIssuer:   "auth-hub",
			BackendTokenAudience: "alt-backend",
		},
	}
	middleware := NewAuthMiddleware(nil, "test-secret", cfg)
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
