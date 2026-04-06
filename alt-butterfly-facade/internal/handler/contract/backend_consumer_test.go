//go:build contract

// Package contract contains Consumer-Driven Contract tests for alt-butterfly-facade → alt-backend.
//
// These tests verify that the BFF's transparent proxy correctly forwards
// Connect-RPC requests to alt-backend and returns responses unchanged.
package contract

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt-butterfly-facade/internal/server"
)

const pactDir = "../../../../pacts"

func newBackendPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-butterfly-facade",
		Provider: "alt-backend",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func createTestToken(t *testing.T, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   uuid.New().String(),
		"email": "test@example.com",
		"role":  role,
		"sid":   "session-123",
		"iss":   "auth-hub",
		"aud":   []string{"alt-backend"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return tokenStr
}

func createBFFHandler(backendURL string) http.Handler {
	cfg := server.Config{
		BackendURL:       backendURL,
		Secret:           []byte("test-secret"),
		Issuer:           "auth-hub",
		Audience:         "alt-backend",
		RequestTimeout:   30 * time.Second,
		StreamingTimeout: 5 * time.Minute,
	}
	return server.NewServerWithTransport(cfg, nil, http.DefaultTransport)
}

func TestBFFProxyUnaryRPC(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("feed stats are available").
		UponReceiving("a GetFeedStats unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.feeds.v2.FeedService/GetFeedStats"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"totalFeeds":    10,
				"totalArticles": 250,
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.feeds.v2.FeedService/GetFeedStats",
				strings.NewReader("{}"),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "totalFeeds")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyAdminRPC(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("knowledge home admin service is available").
		UponReceiving("a GetOverview admin Connect-RPC request with service token").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetOverview"),
			Headers: matchers.MapMatcher{
				"Content-Type":    matchers.String("application/json"),
				"X-Service-Token": matchers.String("service-secret"),
			},
			Body: matchers.MapMatcher{},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"totalEvents": 100,
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			cfg := server.Config{
				BackendURL:       backendURL,
				Secret:           []byte("test-secret"),
				Issuer:           "auth-hub",
				Audience:         "alt-backend",
				RequestTimeout:   30 * time.Second,
				StreamingTimeout: 5 * time.Minute,
				ServiceSecret:    "service-secret",
			}
			handler := server.NewServerWithTransport(cfg, nil, http.DefaultTransport)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetOverview",
				strings.NewReader("{}"),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "admin"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "totalEvents")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyConnectError(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("article does not exist").
		UponReceiving("a Connect-RPC request that returns a not_found error").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.feeds.v2.FeedService/GetFeed"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"feedId": matchers.Like("nonexistent-feed"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 404,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"code":    "not_found",
				"message": "feed not found",
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.feeds.v2.FeedService/GetFeed",
				strings.NewReader(`{"feedId":"nonexistent-feed"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			// BFF should forward the error response as-is
			body, _ := io.ReadAll(recorder.Result().Body)
			assert.Contains(t, string(body), "not_found")
			return nil
		})
	require.NoError(t, err)
}
