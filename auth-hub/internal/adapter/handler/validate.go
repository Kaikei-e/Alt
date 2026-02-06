package handler

import (
	"net/http"

	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// ValidateHandler handles /validate endpoint for nginx auth_request.
type ValidateHandler struct {
	uc *usecase.ValidateSession
}

// NewValidateHandler creates a new validate handler.
func NewValidateHandler(uc *usecase.ValidateSession) *ValidateHandler {
	return &ValidateHandler{uc: uc}
}

// Handle processes the /validate endpoint.
func (h *ValidateHandler) Handle(c echo.Context) error {
	cookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session cookie not found")
	}

	identity, err := h.uc.Execute(c.Request().Context(), cookie.Value)
	if err != nil {
		return mapDomainError(err)
	}

	c.Response().Header().Set("X-Alt-User-Id", identity.UserID)
	c.Response().Header().Set("X-Alt-Tenant-Id", identity.UserID) // Single-tenant
	c.Response().Header().Set("X-Alt-User-Email", identity.Email)
	return c.NoContent(http.StatusOK)
}
