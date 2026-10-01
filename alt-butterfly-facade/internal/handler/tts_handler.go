// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"encoding/binary"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"alt-butterfly-facade/internal/client"
)

// disabledTTSFrame is the pre-encoded Connect end-of-stream envelope for
// failed_precondition ("tts is disabled"). Connect server-streaming returns
// HTTP 200 with application/connect+json and an EOS trailer frame (flag 0x02).
var disabledTTSFrame = func() []byte {
	payload := []byte(`{"error":{"code":"failed_precondition","message":"tts is disabled"}}`)
	buf := make([]byte, 5+len(payload))
	buf[0] = 0x02 // end-of-stream flag
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	return buf
}()

type disabledTTSHandler struct{}

func (h *disabledTTSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/alt.tts.v1.TTSService/SynthesizeStream" {
		http.NotFound(w, r)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/connect+") {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/connect+json")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(disabledTTSFrame)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// NewTTSHandler creates a handler for alt.tts.v1.TTSService.
// Enabled: authenticated transparent streaming proxy reusing ProxyHandler and BackendClient.
// Disabled: Connect failed_precondition ("tts is disabled") without dialing upstream.
func NewTTSHandler(
	enabled bool,
	ttsClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
	streamingTimeout time.Duration,
) http.Handler {
	if enabled && ttsClient == nil {
		panic("ttsClient cannot be nil when tts is enabled")
	}
	if !enabled {
		return &disabledTTSHandler{}
	}
	return NewProxyHandler(
		ttsClient,
		secret,
		issuer,
		audience,
		logger,
		streamingTimeout,
		streamingTimeout,
	)
}
