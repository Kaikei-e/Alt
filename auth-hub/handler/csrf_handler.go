package handler

import (
	"auth-hub/client"
	"auth-hub/config"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// CSRFHandler handles CSRF token requests
type CSRFHandler struct {
	kratosClient *client.KratosClient
	config       *config.Config
}

// NewCSRFHandler creates a new CSRF handler
func NewCSRFHandler(kratosClient *client.KratosClient, cfg *config.Config) *CSRFHandler {
	return &CSRFHandler{
		kratosClient: kratosClient,
		config:       cfg,
	}
}

// CSRFResponse represents the CSRF token response
type CSRFResponse struct {
	Data struct {
		CSRFToken string `json:"csrf_token"`
	} `json:"data"`
}

// generateCSRFToken generates a CSRF token from session ID using HMAC-SHA256
// Returns error if CSRF_SECRET is not configured (security requirement)
func (h *CSRFHandler) generateCSRFToken(sessionID string) (string, error) {
	secret := []byte(h.config.CSRFSecret)
	if len(secret) == 0 {
		// No fallback - CSRF_SECRET must be configured via environment
		return "", errors.New("CSRF_SECRET is not configured")
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sessionID))
	hash := mac.Sum(nil)

	// Return base64-encoded HMAC
	return base64.URLEncoding.EncodeToString(hash), nil
}

// extractSessionID extracts session ID from cookie string
func extractSessionID(cookie string) string {
	// Parse "ory_kratos_session=<session-id>"
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

// Handle processes CSRF token requests
func (h *CSRFHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract session cookie
	sessionCookie := c.Request().Header.Get("Cookie")
	if sessionCookie == "" {
		slog.WarnContext(ctx, "csrf token request without session cookie")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "session cookie required",
		})
	}

	// Validate session with Kratos (existing logic)
	_, err := h.kratosClient.Whoami(ctx, sessionCookie)
	if err != nil {
		slog.ErrorContext(ctx, "failed to validate session", "error", err)
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid session",
		})
	}

	// Extract session ID from cookie
	sessionID := extractSessionID(sessionCookie)
	if sessionID == "" {
		slog.ErrorContext(ctx, "failed to extract session ID from cookie")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "invalid session cookie format",
		})
	}

	// Generate CSRF token from session ID
	csrfToken, err := h.generateCSRFToken(sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate CSRF token", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "CSRF token generation failed",
		})
	}

	// Return successful response
	response := CSRFResponse{}
	response.Data.CSRFToken = csrfToken

	// Log only the first 8 characters of session ID for security
	sessionIDPrefix := sessionID
	if len(sessionID) > 8 {
		sessionIDPrefix = sessionID[:8]
	}
	slog.InfoContext(ctx, "csrf token generated successfully", "session_id_hash", sessionIDPrefix)
	return c.JSON(http.StatusOK, response)
}
