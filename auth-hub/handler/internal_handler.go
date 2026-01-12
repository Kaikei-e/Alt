package handler

import (
	"log/slog"
	"net/http"

	"auth-hub/client"

	"github.com/labstack/echo/v4"
)

// InternalHandler handles internal service-to-service requests
type InternalHandler struct {
	kratosClient *client.KratosClient
}

// NewInternalHandler creates a new internal handler
func NewInternalHandler(kratosClient *client.KratosClient) *InternalHandler {
	return &InternalHandler{
		kratosClient: kratosClient,
	}
}

// SystemUserResponse represents the response for system user endpoint
type SystemUserResponse struct {
	UserID string `json:"user_id"`
}

// HandleSystemUser returns the system user ID for internal service operations
// GET /internal/system-user
func (h *InternalHandler) HandleSystemUser(c echo.Context) error {
	ctx := c.Request().Context()

	userID, err := h.kratosClient.GetFirstIdentityID(ctx)
	if err != nil {
		slog.Error("failed to fetch system user from Kratos",
			"error", err,
			"remote_addr", c.RealIP())
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch system user",
		})
	}

	slog.Info("system user fetched successfully",
		"user_id", userID,
		"remote_addr", c.RealIP())

	return c.JSON(http.StatusOK, SystemUserResponse{
		UserID: userID,
	})
}
