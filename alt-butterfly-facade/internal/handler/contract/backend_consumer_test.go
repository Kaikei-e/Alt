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

// jwtHeaderPattern matches the three dot-separated base64url segments of a
// signed JWT, as forwarded by the BFF in the X-Alt-Backend-Token header.
const jwtHeaderPattern = `^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`

// jwtHeaderExample is the pact example value for jwtHeaderPattern. It is
// deliberately not base64url-of-JSON: a realistic "eyJ..." literal is
// indistinguishable from a leaked token to secret scanners.
const jwtHeaderExample = "header.payload.signature"

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
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
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
		UponReceiving("a GetOverview admin Connect-RPC request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetOverview"),
			// No X-Alt-Backend-Token here, unlike the user-token routes:
			// admin RPCs go through BackendClient.ForwardServiceRequest,
			// which strips the caller's token and relies on the mTLS
			// transport for service-to-service auth.
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
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
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

// TestBFFProxyBatchPrefetchArticleContent records the BFF's expectation of
// alt.articles.v2.ArticleService/BatchPrefetchArticleContent — the
// fire-and-forget warm the reader issues for the next few items it thinks the
// user is about to open.
//
// It is recorded here, on the consumer side, because CLAUDE.md rule 7 makes a
// Pact CDC test the gate on any new cross-service RPC: "proto compiled + E2E
// green" cannot tell a wired producer from a producer whose DI silently
// handed it nothing (ADR-000928).
//
// Two properties of the shape matter and are pinned deliberately:
//
//   - The response is an *acceptance receipt*, not a fetch result. The warm
//     runs after the RPC returns, so the body can only say how many URLs
//     claimed a slot. A client that treats a 200 here as "the body is cached"
//     is reading a promise this contract does not make.
//   - Only acceptedCount is asserted. connect-go marshals with protojson's
//     default options, which omit zero-valued scalars, so shedCount and
//     rejectedCount are absent from the wire whenever they are zero. Pinning
//     them at 0 would pin an encoder detail rather than the contract.
func TestBFFProxyBatchPrefetchArticleContent(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("article content prefetch is enabled").
		UponReceiving("a BatchPrefetchArticleContent fire-and-forget warm proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.articles.v2.ArticleService/BatchPrefetchArticleContent"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"urls": matchers.EachLike("https://example.com/next-article", 1),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"acceptedCount": 2,
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.articles.v2.ArticleService/BatchPrefetchArticleContent",
				strings.NewReader(`{"urls":["https://example.com/next-article","https://example.org/the-one-after"]}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "acceptedCount")
			return nil
		})
	require.NoError(t, err)
}
