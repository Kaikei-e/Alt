package handler

import (
	"net/http"
	"time"

	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// SessionHandler handles /session endpoint returning JSON for the frontend.
type SessionHandler struct {
	uc               *usecase.GetSession
	authSharedSecret string
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(uc *usecase.GetSession, authSharedSecret string) *SessionHandler {
	return &SessionHandler{uc: uc, authSharedSecret: authSharedSecret}
}

// sessionUser represents the user object in the response.
type sessionUser struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
	LastLoginAt time.Time `json:"lastLoginAt,omitempty"`
}

// sessionInfo represents the session object in the response.
type sessionInfo struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}

// sessionResponse represents the JSON response structure.
type sessionResponse struct {
	OK      bool        `json:"ok"`
	User    sessionUser `json:"user"`
	Session sessionInfo `json:"session"`
}

// Handle processes the /session endpoint and returns JSON.
func (h *SessionHandler) Handle(c echo.Context) error {
	cookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session cookie not found")
	}

	result, err := h.uc.Execute(c.Request().Context(), cookie.Value)
	if err != nil {
		return mapDomainError(err)
	}

	c.Response().Header().Set("X-Alt-Backend-Token", result.BackendToken)

	// Legacy: shared secret header for backward compatibility
	if h.authSharedSecret != "" {
		c.Response().Header().Set("X-Alt-Shared-Secret", h.authSharedSecret)
	}

	return c.JSON(http.StatusOK, sessionResponse{
		OK: true,
		User: sessionUser{
			ID:          result.UserID,
			TenantID:    result.TenantID,
			Email:       result.Email,
			Role:        result.Role,
			CreatedAt:   result.CreatedAt,
			LastLoginAt: time.Now(),
		},
		Session: sessionInfo{
			ID:     result.SessionID,
			Active: true,
		},
	})
}
