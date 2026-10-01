// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"log/slog"
	"net/http"
	"time"

	"alt-butterfly-facade/internal/client"
)

// NewTTSHandler creates a handler for alt.tts.v1.TTSService.
// Minimal stub for TDD RED phase: panics until implemented.
func NewTTSHandler(
	enabled bool,
	ttsClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
	streamingTimeout time.Duration,
) http.Handler {
	panic("not implemented")
}
