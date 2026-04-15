//go:build contract

// Package contract contains Consumer-Driven Contract tests for alt-butterfly-facade → tts-speaker.
//
// These tests verify that the BFF correctly routes TTS Connect-RPC requests
// to tts-speaker. Authentication is established at the transport layer.
package contract

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/internal/server"
)

func newTTSPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-butterfly-facade",
		Provider: "tts-speaker",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func TestBFFProxyTTSSynthesize(t *testing.T) {
	mockProvider := newTTSPact(t)

	err := mockProvider.
		AddInteraction().
		Given("TTS service is available").
		UponReceiving("a Synthesize request forwarded by BFF with service token").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.tts.v1.TTSService/Synthesize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"text": matchers.Like("Hello world"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"audioWav": "",
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			ttsURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)

			// Create a dummy alt-backend that should NOT receive TTS requests
			altBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("TTS request should not reach alt-backend")
			}))
			defer altBackend.Close()

			cfg := server.Config{
				BackendURL:       altBackend.URL,
				TTSConnectURL:    ttsURL,
				Secret:           []byte("test-secret"),
				Issuer:           "auth-hub",
				Audience:         "alt-backend",
				RequestTimeout:   30 * time.Second,
				StreamingTimeout: 5 * time.Minute,
			}
			handler := server.NewServerWithTransport(cfg, nil, http.DefaultTransport)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.tts.v1.TTSService/Synthesize",
				strings.NewReader(`{"text":"Hello world"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "audioWav")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyTTSError(t *testing.T) {
	mockProvider := newTTSPact(t)

	err := mockProvider.
		AddInteraction().
		Given("TTS service encounters an error").
		UponReceiving("a Synthesize request that returns a service error").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.tts.v1.TTSService/Synthesize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"text": matchers.Like(""),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 400,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"code":    "invalid_argument",
				"message": "text must not be empty",
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			ttsURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)

			altBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("TTS request should not reach alt-backend")
			}))
			defer altBackend.Close()

			cfg := server.Config{
				BackendURL:       altBackend.URL,
				TTSConnectURL:    ttsURL,
				Secret:           []byte("test-secret"),
				Issuer:           "auth-hub",
				Audience:         "alt-backend",
				RequestTimeout:   30 * time.Second,
				StreamingTimeout: 5 * time.Minute,
			}
			handler := server.NewServerWithTransport(cfg, nil, http.DefaultTransport)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.tts.v1.TTSService/Synthesize",
				strings.NewReader(`{"text":""}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Contains(t, recorder.Body.String(), "invalid_argument")
			return nil
		})
	require.NoError(t, err)
}
