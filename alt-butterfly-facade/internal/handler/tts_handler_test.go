package handler

import (
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTTSHandler_PanicsWhenEnabledWithNilClient(t *testing.T) {
	assert.Panics(t, func() {
		NewTTSHandler(
			true,
			nil,
			testPublicKey,
			"auth-hub",
			"alt-backend",
			nil,
			5*time.Minute,
		)
	}, "NewTTSHandler must panic when enabled=true but client is nil (rule 8)")
}

func TestDisabledTTSHandler_ExactFrame(t *testing.T) {
	h := NewTTSHandler(
		false,
		nil,
		testPublicKey,
		"auth-hub",
		"alt-backend",
		nil,
		5*time.Minute,
	)

	payload := []byte(`{"error":{"code":"failed_precondition","message":"tts is disabled"}}`)
	require.LessOrEqual(t, len(payload), math.MaxUint32)
	expectedBytes := make([]byte, 5+len(payload))
	expectedBytes[0] = 0x02
	// #nosec G115 -- bounds-checked against math.MaxUint32 above.
	binary.BigEndian.PutUint32(expectedBytes[1:5], uint32(len(payload)))
	copy(expectedBytes[5:], payload)

	t.Run("echoes application/connect+proto content type and writes exact frame", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/alt.tts.v1.TTSService/SynthesizeStream", nil)
		req.Header.Set("Content-Type", "application/connect+proto")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/connect+proto", rec.Header().Get("Content-Type"))
		assert.Equal(t, expectedBytes, rec.Body.Bytes())
	})

	t.Run("echoes application/connect+json content type and writes exact frame", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/alt.tts.v1.TTSService/SynthesizeStream", nil)
		req.Header.Set("Content-Type", "application/connect+json")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/connect+json", rec.Header().Get("Content-Type"))
		assert.Equal(t, expectedBytes, rec.Body.Bytes())
	})

	t.Run("defaults to application/connect+json when content type is not connect", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/alt.tts.v1.TTSService/SynthesizeStream", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/connect+json", rec.Header().Get("Content-Type"))
		assert.Equal(t, expectedBytes, rec.Body.Bytes())
	})

	t.Run("returns 404 for any path other than /alt.tts.v1.TTSService/SynthesizeStream", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/alt.tts.v1.TTSService/UnrecognizedMethod", nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
