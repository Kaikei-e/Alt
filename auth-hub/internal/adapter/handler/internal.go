package handler

import (
	"log/slog"
	"net/http"

	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// InternalHandler handles internal service-to-service requests.
type InternalHandler struct {
	uc *usecase.GetSystemUser
}

// NewInternalHandler creates a new internal handler.
func NewInternalHandler(uc *usecase.GetSystemUser) *InternalHandler {
	return &InternalHandler{uc: uc}
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
