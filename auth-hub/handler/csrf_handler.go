package handler

import (
	"auth-hub/client"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

// CSRFHandler handles CSRF token requests
type CSRFHandler struct {
	kratosClient *client.KratosClient
}

// NewCSRFHandler creates a new CSRF handler
func NewCSRFHandler(kratosClient *client.KratosClient) *CSRFHandler {
	return &CSRFHandler{
		kratosClient: kratosClient,
	}
}

// CSRFResponse represents the CSRF token response
type CSRFResponse struct {
	Data struct {
		CSRFToken string `json:"csrf_token"`
	} `json:"data"`
}

// generateCSRFToken generates a CSRF token from session ID using HMAC-SHA256
func (h *CSRFHandler) generateCSRFToken(sessionID string) string {
	// Use a server-side secret (from env or config)
	secret := []byte(os.Getenv("CSRF_SECRET"))
	if len(secret) == 0 {
		// Fallback to a default secret (for development only)
		secret = []byte("development-csrf-secret-change-in-production")
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sessionID))
	hash := mac.Sum(nil)

	// Return base64-encoded HMAC
	return base64.URLEncoding.EncodeToString(hash)
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
	// Extract session cookie
	sessionCookie := c.Request().Header.Get("Cookie")
	if sessionCookie == "" {
		slog.Warn("csrf token request without session cookie")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "session cookie required",
		})
	}

	// Validate session with Kratos (existing logic)
	_, err := h.kratosClient.Whoami(c.Request().Context(), sessionCookie)
	if err != nil {
		slog.Error("failed to validate session", "error", err)
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid session",
		})
	}

	// Extract session ID from cookie
	sessionID := extractSessionID(sessionCookie)
	if sessionID == "" {
		slog.Error("failed to extract session ID from cookie")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "invalid session cookie format",
		})
	}

	// Generate CSRF token from session ID
	csrfToken := h.generateCSRFToken(sessionID)

	// Return successful response
	response := CSRFResponse{}
	response.Data.CSRFToken = csrfToken

	// Log only the first 8 characters of session ID for security
	sessionIDPrefix := sessionID
	if len(sessionID) > 8 {
		sessionIDPrefix = sessionID[:8]
	}
	slog.Info("csrf token generated successfully", "session_id_hash", sessionIDPrefix)
	return c.JSON(http.StatusOK, response)
}
