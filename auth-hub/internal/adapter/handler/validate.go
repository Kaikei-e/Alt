package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"auth-hub/internal/domain"
	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// ValidateHandler handles /validate endpoint for nginx auth_request.
type ValidateHandler struct {
	uc    *usecase.ValidateSession
	token domain.TokenIssuer
}

// NewValidateHandler creates a new validate handler.
// token must be non-nil: /validate is the sole source of the backend JWT that
// nginx forwards downstream, so an unwired TokenIssuer is a startup bug, not a
// degraded mode.
func NewValidateHandler(uc *usecase.ValidateSession, token domain.TokenIssuer) *ValidateHandler {
	if token == nil {
		panic("handler: NewValidateHandler requires a non-nil TokenIssuer")
	}
	return &ValidateHandler{uc: uc, token: token}
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

	// Issue JWT backend token for nginx to forward to backend. A failure here
	// must fail closed: without this header the backend receives no proof of
	// authentication, so returning 200 would let an unauthenticated request through.
	backendToken, tokenErr := h.token.IssueBackendToken(identity, identity.SessionID)
	if tokenErr != nil {
		slog.ErrorContext(c.Request().Context(), "failed to issue backend token in validate", "error", tokenErr)
		return mapDomainError(fmt.Errorf("%w: %w", domain.ErrTokenGeneration, tokenErr))
	}
	c.Response().Header().Set("X-Alt-Backend-Token", backendToken)

	c.Response().Header().Set("X-Alt-User-Id", identity.UserID)
	c.Response().Header().Set("X-Alt-Tenant-Id", identity.UserID) // Single-tenant
	c.Response().Header().Set("X-Alt-User-Email", identity.Email)
	return c.NoContent(http.StatusOK)
}
