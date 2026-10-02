package handler

import (
	"log/slog"
	"net/http"

	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// InternalHandler handles internal service-to-service requests.
type InternalHandler struct {
	uc           *usecase.GetSystemUser
	invalidateUC *usecase.InvalidateSession
	introspectUC *usecase.IntrospectToken
}

// NewInternalHandler creates a new internal handler.
func NewInternalHandler(uc *usecase.GetSystemUser, invalidateUC *usecase.InvalidateSession, introspectUC *usecase.IntrospectToken) *InternalHandler {
	return &InternalHandler{uc: uc, invalidateUC: invalidateUC, introspectUC: introspectUC}
}

// InvalidateSessionRequest represents the payload for internal session invalidation.
type InvalidateSessionRequest struct {
	Cookie string `json:"cookie"`
}

// systemUserResponse represents the response for system user endpoint.
type systemUserResponse struct {
	UserID string `json:"user_id"`
}

// HandleSystemUser returns the system user ID for internal service operations.
func (h *InternalHandler) HandleSystemUser(c echo.Context) error {
	ctx := c.Request().Context()

	userID, err := h.uc.Execute(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch system user", "error", err, "remote_addr", c.RealIP())
		return mapDomainError(err)
	}

	slog.InfoContext(ctx, "system user fetched", "user_id", userID, "remote_addr", c.RealIP())
	return c.JSON(http.StatusOK, systemUserResponse{UserID: userID})
}

// HandleInvalidateSession purges a session from the cache on request from internal services.
func (h *InternalHandler) HandleInvalidateSession(c echo.Context) error {
	ctx := c.Request().Context()
	var req InvalidateSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Cookie == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "cookie is required")
	}

	if h.invalidateUC == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session invalidation not wired")
	}

	if err := h.invalidateUC.Execute(ctx, req.Cookie); err != nil {
		return mapDomainError(err)
	}

	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// IntrospectTokenRequest represents the request body for token introspection.
type IntrospectTokenRequest struct {
	Token string `json:"token"`
}

// HandleIntrospectToken validates a JWT token and returns its key claims.
// This endpoint is only accessible to verified mTLS peers on the internal listener.
func (h *InternalHandler) HandleIntrospectToken(c echo.Context) error {
	ctx := c.Request().Context()

	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 4096)
	var req IntrospectTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if h.introspectUC == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "token introspection not wired")
	}

	res, err := h.introspectUC.Execute(ctx, req.Token)
	if err != nil {
		// Log generically without logging the raw token
		slog.WarnContext(ctx, "token introspection failed", "error", err)
		// Always return 200 with Active: false on invalid tokens, per typical introspection APIs
		return c.JSON(http.StatusOK, map[string]interface{}{"active": false})
	}

	return c.JSON(http.StatusOK, res)
}
