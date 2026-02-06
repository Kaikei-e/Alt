package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"auth-hub/cache"
	"auth-hub/client"
	"auth-hub/config"
	"auth-hub/token"

	"github.com/labstack/echo/v4"
)

// SessionHandler handles /session endpoint for frontend JSON responses
type SessionHandler struct {
	kratosClient     KratosClient
	sessionCache     *cache.SessionCache
	authSharedSecret string
	config           *config.Config
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(kratosClient KratosClient, sessionCache *cache.SessionCache, authSharedSecret string, cfg *config.Config) *SessionHandler {
	return &SessionHandler{
		kratosClient:     kratosClient,
		sessionCache:     sessionCache,
		authSharedSecret: authSharedSecret,
		config:           cfg,
	}
}

// User represents the user object in the response (matching frontend expectation)
type User struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
	LastLoginAt time.Time `json:"lastLoginAt,omitempty"`
}

// Session represents the session object in the response
type Session struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}

// SessionResponse represents the JSON response structure
type SessionResponse struct {
	OK      bool    `json:"ok"`
	User    User    `json:"user"`
	Session Session `json:"session"`
}

// Handle processes the /session endpoint and returns JSON
func (h *SessionHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract session cookie
	cookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session cookie not found")
	}

	sessionID := cookie.Value

	// Check cache first
	if entry, found := h.sessionCache.Get(sessionID); found {
		// Cache hit - return cached identity as JSON
		// Note: CreatedAt from cache may not be accurate, but sufficient for session validation

		// Create identity from cache entry for JWT generation
		identity := &client.Identity{
			ID:        entry.UserID,
			Email:     entry.Email,
			CreatedAt: time.Now().Add(-24 * time.Hour), // Approximate
			SessionID: sessionID,
		}

		// Generate backend token
		backendToken, err := token.IssueBackendToken(h.config, identity, sessionID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to issue backend token", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to issue backend token")
		}

		// Add backend token to response header
		c.Response().Header().Set("X-Alt-Backend-Token", backendToken)

		// Legacy: Add shared secret header if configured (for backward compatibility during migration)
		if h.authSharedSecret != "" {
			c.Response().Header().Set("X-Alt-Shared-Secret", h.authSharedSecret)
		}

		response := SessionResponse{
			OK: true,
			User: User{
				ID:          entry.UserID,
				TenantID:    entry.TenantID,
				Email:       entry.Email,
				Role:        "user",                          // Default role
				CreatedAt:   time.Now().Add(-24 * time.Hour), // Approximate (cache doesn't store CreatedAt)
				LastLoginAt: time.Now(),                      // Current session validation time
			},
			Session: Session{
				ID:     sessionID, // Session ID is the cookie value, not identity.ID
				Active: true,
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Cache miss - validate with Kratos
	fullCookie := fmt.Sprintf("ory_kratos_session=%s", sessionID)
	identity, err := h.kratosClient.Whoami(ctx, fullCookie)
	if err != nil {
		// Check if it's an authentication error (401) or service error (502)
		if strings.Contains(err.Error(), "authentication failed") {
			return echo.NewHTTPError(http.StatusUnauthorized, "session validation failed")
		}
		return echo.NewHTTPError(http.StatusBadGateway, "failed to validate session")
	}

	// Cache the validated session
	// Using UserID as TenantID (single-tenant architecture)
	h.sessionCache.Set(sessionID, identity.ID, identity.ID, identity.Email)

	// Generate backend token
	backendToken, err := token.IssueBackendToken(h.config, identity, identity.SessionID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to issue backend token", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to issue backend token")
	}

	// Add backend token to response header
	c.Response().Header().Set("X-Alt-Backend-Token", backendToken)

	// Legacy: Add shared secret header if configured (for backward compatibility during migration)
	if h.authSharedSecret != "" {
		c.Response().Header().Set("X-Alt-Shared-Secret", h.authSharedSecret)
	}

	// Return identity as JSON
	response := SessionResponse{
		OK: true,
		User: User{
			ID:          identity.ID,
			TenantID:    identity.ID,
			Email:       identity.Email,
			Role:        "user", // Default role
			CreatedAt:   identity.CreatedAt,
			LastLoginAt: time.Now(), // Current session validation time
		},
		Session: Session{
			ID:     identity.SessionID, // Kratos session ID, not identity.ID
			Active: true,
		},
	}

	return c.JSON(http.StatusOK, response)
}
