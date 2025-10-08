package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"auth-hub/cache"

	"github.com/labstack/echo/v4"
)

// SessionHandler handles /session endpoint for frontend JSON responses
type SessionHandler struct {
	kratosClient KratosClient
	sessionCache *cache.SessionCache
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(kratosClient KratosClient, sessionCache *cache.SessionCache) *SessionHandler {
	return &SessionHandler{
		kratosClient: kratosClient,
		sessionCache: sessionCache,
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
		response := SessionResponse{
			OK: true,
			User: User{
				ID:          entry.UserID,
				TenantID:    entry.TenantID,
				Email:       entry.Email,
				Role:        "user", // Default role
				CreatedAt:   time.Now().Add(-24 * time.Hour), // Approximate (cache doesn't store CreatedAt)
				LastLoginAt: time.Now(), // Current session validation time
			},
			Session: Session{
				ID:     entry.UserID,
				Active: true,
			},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Cache miss - validate with Kratos
	fullCookie := fmt.Sprintf("ory_kratos_session=%s", sessionID)
	identity, err := h.kratosClient.Whoami(c.Request().Context(), fullCookie)
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
			ID:     identity.ID,
			Active: true,
		},
	}

	return c.JSON(http.StatusOK, response)
}
