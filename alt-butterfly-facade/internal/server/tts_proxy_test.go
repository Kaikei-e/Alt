package server

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createConnectEnvelope builds a 5-byte prefixed Connect streaming envelope:
// 1 byte flags (0x00 data, 0x02 end-of-stream) + 4 bytes big-endian length + payload.
func createConnectEnvelope(flag byte, payload []byte) []byte {
	buf := make([]byte, 5+len(payload))
	buf[0] = flag
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	return buf
}

// TestTTSProxy_StreamingPassThrough pins requirement 4(a):
// A fake upstream writes 3 enveloped frames with pauses; the test reads the BFF
// response incrementally and observes the first frame before the upstream finished (flush),
// and the bytes arrive unmodified.
func TestTTSProxy_StreamingPassThrough(t *testing.T) {
	upstreamDone := make(chan struct{})

	frame1Payload := []byte(`{"chunk":1,"audio_wav":"Y2h1bmsx"}`)
	frame2Payload := []byte(`{"chunk":2,"audio_wav":"Y2h1bmsy"}`)
	frame3Payload := []byte(`{}`) // end-of-stream trailer frame

	frame1 := createConnectEnvelope(0x00, frame1Payload)
	frame2 := createConnectEnvelope(0x00, frame2Payload)
	frame3 := createConnectEnvelope(0x02, frame3Payload)

	allExpectedBytes := append(append(append([]byte{}, frame1...), frame2...), frame3...)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alt.tts.v1.TTSService/SynthesizeStream", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NotEmpty(t, r.Header.Get("X-Alt-Backend-Token"), "token must be forwarded to upstream")

		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "upstream must support flushing")

		// Write frame 1 and flush immediately
		_, err := w.Write(frame1)
		require.NoError(t, err)
		flusher.Flush()

		// Pause between frame 1 and frame 2 to allow client to read incrementally
		select {
		case <-time.After(150 * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		// Write frame 2 and flush
		_, err = w.Write(frame2)
		require.NoError(t, err)
		flusher.Flush()

		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		// Write frame 3 (end-of-stream) and flush
		_, err = w.Write(frame3)
		require.NoError(t, err)
		flusher.Flush()
		close(upstreamDone)
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    upstream.URL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"こんにちは"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/connect+json")

	// Read frame 1 header (5 bytes)
	f1Header := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, f1Header)
	require.NoError(t, err)
	assert.Equal(t, byte(0x00), f1Header[0], "frame 1 flag must be 0x00 (data)")
	f1Len := binary.BigEndian.Uint32(f1Header[1:5])
	f1Body := make([]byte, f1Len)
	_, err = io.ReadFull(resp.Body, f1Body)
	require.NoError(t, err)
	assert.Equal(t, frame1Payload, f1Body)

	// Flush verification: frame 1 must be received BEFORE upstream finishes sending remaining frames
	select {
	case <-upstreamDone:
		t.Fatal("frame 1 should have been received before upstream finished (response was buffered, not flushed)")
	default:
		// Upstream still paused/sending: incremental flush verified
	}

	// Read remaining bytes and verify full stream arrives unmodified
	remainingBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	fullReceived := append(append(f1Header, f1Body...), remainingBytes...)
	assert.Equal(t, allExpectedBytes, fullReceived, "streaming bytes must arrive completely unmodified")
}

// TestTTSProxy_AuthRequired pins requirement 4(b):
// Missing or invalid token yields 401 Unauthorized, and the upstream is not called.
func TestTTSProxy_AuthRequired(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		t.Fatalf("upstream must not be called when authentication fails")
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    upstream.URL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	t.Run("missing token", func(t *testing.T) {
		upstreamCalled = false
		req, err := http.NewRequest(
			http.MethodPost,
			bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
			strings.NewReader(`{"text":"test"}`),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/connect+json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.False(t, upstreamCalled, "upstream must not be called for missing token")
	})

	t.Run("invalid token", func(t *testing.T) {
		upstreamCalled = false
		req, err := http.NewRequest(
			http.MethodPost,
			bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
			strings.NewReader(`{"text":"test"}`),
		)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/connect+json")
		req.Header.Set("X-Alt-Backend-Token", "invalid-token-string")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.False(t, upstreamCalled, "upstream must not be called for invalid token")
	})
}

// TestTTSProxy_DisabledMode pins requirement 4(c):
// In disabled mode, every request answers a Connect error failed_precondition with message
// "tts is disabled" (HTTP 200 with application/connect+json and an end-of-stream frame carrying the error),
// and never dials upstream.
func TestTTSProxy_DisabledMode(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		t.Fatalf("upstream must never be called in disabled mode")
	}))
	defer upstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "disabled",
		TTSConnectURL:    upstream.URL, // even if URL is configured, disabled mode must not dial it
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"test"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Connect streaming errors return HTTP 200 with application/connect+json
	// and the error is carried inside the end-of-stream frame.
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/connect+json")
	assert.False(t, upstreamCalled, "upstream must never be called in disabled mode")

	// Read the end-of-stream frame: 5-byte prefix + JSON payload
	eosHeader := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, eosHeader)
	require.NoError(t, err)
	assert.Equal(t, byte(0x02), eosHeader[0], "flag must be 0x02 (end-of-stream)")

	eosLen := binary.BigEndian.Uint32(eosHeader[1:5])
	eosPayload := make([]byte, eosLen)
	_, err = io.ReadFull(resp.Body, eosPayload)
	require.NoError(t, err)

	var eosMessage struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(eosPayload, &eosMessage), "payload must be valid JSON: %s", string(eosPayload))
	assert.Equal(t, "failed_precondition", eosMessage.Error.Code)
	assert.Equal(t, "tts is disabled", eosMessage.Error.Message)
}

// TestTTSProxy_UnreachableUpstream pins requirement 4(f):
// When enabled and upstream is unreachable, the client receives a Connect unavailable error.
func TestTTSProxy_UnreachableUpstream(t *testing.T) {
	// Create and immediately close a listener to get an unreachable port
	closedUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := closedUpstream.URL
	closedUpstream.Close()

	secret := []byte("test-secret-at-least-32-chars-long!")
	cfg := Config{
		BackendURL:       "http://127.0.0.1:1",
		TTSProxy:         "enabled",
		TTSConnectURL:    unreachableURL,
		Secret:           secret,
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}

	bffSrv := httptest.NewServer(NewServerWithTransport(cfg, nil, http.DefaultTransport))
	defer bffSrv.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		bffSrv.URL+"/alt.tts.v1.TTSService/SynthesizeStream",
		strings.NewReader(`{"text":"test"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("X-Alt-Backend-Token", createValidToken(t, secret))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// An unreachable upstream maps to Connect unavailable.
	// In the Connect protocol, this surfaces as HTTP 502 (Bad Gateway) or HTTP 503 (Service Unavailable).
	assert.True(t,
		resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable,
		"expected HTTP 502 Bad Gateway or 503 Service Unavailable mapping to Connect unavailable; got %d",
		resp.StatusCode,
	)

	// If body contains JSON error, code must be unavailable
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if bytes.Contains(bodyBytes, []byte(`"code"`)) {
		var errResp struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Code != "" {
			assert.Equal(t, "unavailable", errResp.Code)
		}
	}
}
