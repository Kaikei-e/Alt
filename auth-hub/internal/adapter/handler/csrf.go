package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"auth-hub/internal/usecase"

	"github.com/labstack/echo/v4"
)

// CSRFHandler handles CSRF token requests.
type CSRFHandler struct {
	uc *usecase.GenerateCSRF
}

// NewCSRFHandler creates a new CSRF handler.
func NewCSRFHandler(uc *usecase.GenerateCSRF) *CSRFHandler {
	return &CSRFHandler{uc: uc}
}

// csrfResponse represents the CSRF token response.
type csrfResponse struct {
	Data struct {
		CSRFToken string `json:"csrf_token"`
	} `json:"data"`
}

// Handle processes CSRF token requests.
func (h *CSRFHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	rawCookie := c.Request().Header.Get("Cookie")
	if rawCookie == "" {
		slog.WarnContext(ctx, "csrf token request without session cookie")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "session cookie required",
		})
	}

	sessionID := extractSessionID(rawCookie)

	token, err := h.uc.Execute(ctx, rawCookie, sessionID)
	if err != nil {
		return mapDomainError(err)
	}

	// Do not log Cookie-derived session material (even truncated) — clear-text logging.
	slog.InfoContext(ctx, "csrf token generated", "session_present", sessionID != "")

	resp := csrfResponse{}
	resp.Data.CSRFToken = token
	return c.JSON(http.StatusOK, resp)
}

// extractSessionID extracts session ID from cookie string.
func extractSessionID(cookie string) string {
	prefix := "ory_kratos_session="
	start := strings.Index(cookie, prefix)
	if start == -1 {
		return ""
	}

	start += len(prefix)
	end := strings.Index(cookie[start:], ";")
	if end == -1 {
		return cookie[start:]
	}
	return cookie[start : start+end]
}
