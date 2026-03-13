package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"auth-hub/cache"
	"auth-hub/client"
	"auth-hub/config"
	"auth-hub/token"

	"github.com/labstack/echo/v4"
)

// KratosClient interface for dependency injection
type KratosClient interface {
	Whoami(ctx context.Context, cookie string) (*client.Identity, error)
}

// ValidateHandler handles session validation requests
type ValidateHandler struct {
	kratosClient KratosClient
	sessionCache *cache.SessionCache
	config       *config.Config
}

// NewValidateHandler creates a new validate handler
func NewValidateHandler(kratosClient KratosClient, sessionCache *cache.SessionCache, cfg *config.Config) *ValidateHandler {
	return &ValidateHandler{
		kratosClient: kratosClient,
		sessionCache: sessionCache,
		config:       cfg,
	}
}

// Handle processes the /validate endpoint
func (h *ValidateHandler) Handle(c echo.Context) error {
	// Extract session cookie
	cookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session cookie not found")
	}

	sessionID := cookie.Value

	// Check cache first
	if entry, found := h.sessionCache.Get(sessionID); found {
		// Cache hit - return cached identity
		c.Response().Header().Set("X-Alt-User-Id", entry.UserID)
		c.Response().Header().Set("X-Alt-Tenant-Id", entry.TenantID)
		c.Response().Header().Set("X-Alt-User-Email", entry.Email)

		// Issue JWT backend token
		identity := &client.Identity{ID: entry.UserID, Email: entry.Email, SessionID: sessionID}
		backendToken, err := token.IssueBackendToken(h.config, identity, sessionID)
		if err != nil {
			slog.ErrorContext(c.Request().Context(), "failed to issue backend token in validate", "error", err)
		} else {
			c.Response().Header().Set("X-Alt-Backend-Token", backendToken)
		}

		return c.NoContent(http.StatusOK)
	}

	// Cache miss - validate with Kratos
	fullCookie := fmt.Sprintf("ory_kratos_session=%s", sessionID)
	identity, err := h.kratosClient.Whoami(c.Request().Context(), fullCookie)
	if err != nil {
		// Check if it's an authentication error (401) or service error (500)
		if strings.Contains(err.Error(), "authentication failed") {
			return echo.NewHTTPError(http.StatusUnauthorized, "session validation failed")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to validate session")
	}

	// Cache the validated session
	// Using UserID as TenantID (single-tenant architecture)
	h.sessionCache.Set(sessionID, identity.ID, identity.ID, identity.Email)

	// Issue JWT backend token
	backendToken, tokenErr := token.IssueBackendToken(h.config, identity, identity.SessionID)
	if tokenErr != nil {
		slog.ErrorContext(c.Request().Context(), "failed to issue backend token in validate", "error", tokenErr)
	} else {
		c.Response().Header().Set("X-Alt-Backend-Token", backendToken)
	}

	// Return identity headers
	c.Response().Header().Set("X-Alt-User-Id", identity.ID)
	c.Response().Header().Set("X-Alt-Tenant-Id", identity.ID)
	c.Response().Header().Set("X-Alt-User-Email", identity.Email)

	return c.NoContent(http.StatusOK)
}
