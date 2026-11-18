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

const (
	uuidStringPattern       = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	rfc3339TimestampPattern = `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`
)

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
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tokenStr, err := token.SignedString(testPrivateKey)
	require.NoError(t, err)
	return tokenStr
}

func createBFFHandler(backendURL string) http.Handler {
	cfg := server.Config{
		BackendURL:       backendURL,
		TokenVerifyKey:   testPublicKey,
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
			Headers: matchers.MapMatcher{
				"Content-Type":  matchers.String("application/json"),
				"Authorization": matchers.Regex("Bearer test-operator-token", `^Bearer .+$`),
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
				BackendURL:           backendURL,
				BackendInternalURL:   backendURL,
				BackendOperatorToken: "test-operator-token",
				TokenVerifyKey:       testPublicKey,
				Issuer:               "auth-hub",
				Audience:             "alt-backend",
				RequestTimeout:       30 * time.Second,
				StreamingTimeout:     5 * time.Minute,
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

// clusterId, sourceIndex and positions are proto3 scalars that protojson
// omits at 0, so the contract does not pin them.
func TestBFFProxyFetchTagCloud(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("tag cloud data with co-occurring tags is available").
		UponReceiving("a FetchTagCloud unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.articles.v2.ArticleService/FetchTagCloud"),
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
			Body: matchers.MapMatcher{
				"tags": matchers.EachLike(map[string]interface{}{
					"tagName":      matchers.Like("rust"),
					"articleCount": matchers.Like(42),
				}, 1),
				"edges": matchers.EachLike(map[string]interface{}{
					"targetIndex": matchers.Like(1),
					"weight":      matchers.Like(0.85),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.articles.v2.ArticleService/FetchTagCloud",
				strings.NewReader("{}"),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "edges")
			return nil
		})
	require.NoError(t, err)
}

// duplicates is a proto3 scalar that protojson omits at 0, so the contract
// pins only accepted on the 200 response.
func TestBFFProxyRecordReadingEvents(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading service is available").
		UponReceiving("a RecordReadingEvents unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/RecordReadingEvents"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"events": matchers.EachLike(map[string]interface{}{
					"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000001", uuidStringPattern),
					"occurredAt":    matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
					"sessionEnded": map[string]interface{}{
						"articleId":       matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
						"surface":         matchers.String("READING_SURFACE_GALLERY"),
						"activeDwellMs":   matchers.Like(45000),
						"maxScrollRatio":  matchers.Like(0.85),
						"bodyLengthChars": matchers.Like(2400),
						"reachedEnd":      matchers.Like(true),
					},
				}, 1),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"accepted": matchers.Integer(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/RecordReadingEvents",
				strings.NewReader(`{"events":[{"clientEventId":"01960000-0000-7000-8000-000000000002","occurredAt":"2026-11-18T01:23:45Z","sessionEnded":{"articleId":"22222222-2222-4222-8222-222222222222","surface":"READING_SURFACE_GALLERY","activeDwellMs":32000,"maxScrollRatio":0.92,"bodyLengthChars":1800,"reachedEnd":false}}]}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "accepted")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyRecordReadingEvents_OpenedAndReadArticleIDs(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading service records opened and marks qualifying sessions as read").
		UponReceiving("a RecordReadingEvents unary Connect-RPC request with opened and sessionEnded returning readArticleIds").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/RecordReadingEvents"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"events": matchers.Like([]interface{}{
					map[string]interface{}{
						"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000011", uuidStringPattern),
						"occurredAt":    matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
						"opened": map[string]interface{}{
							"articleId": matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
							"surface":   matchers.String("READING_SURFACE_GALLERY"),
						},
					},
					map[string]interface{}{
						"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000012", uuidStringPattern),
						"occurredAt":    matchers.Regex("2026-11-18T00:01:00Z", rfc3339TimestampPattern),
						"sessionEnded": map[string]interface{}{
							"articleId":       matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
							"surface":         matchers.String("READING_SURFACE_GALLERY"),
							"activeDwellMs":   matchers.Like(10000),
							"maxScrollRatio":  matchers.Like(1.0),
							"bodyLengthChars": matchers.Like(500),
							"reachedEnd":      matchers.Like(true),
						},
					},
				}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"accepted":       matchers.Integer(2),
				"readArticleIds": matchers.EachLike("11111111-1111-4111-8111-111111111111", 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/RecordReadingEvents",
				strings.NewReader(`{"events":[{"clientEventId":"01960000-0000-7000-8000-000000000011","occurredAt":"2026-11-18T00:00:00Z","opened":{"articleId":"11111111-1111-4111-8111-111111111111","surface":"READING_SURFACE_GALLERY"}},{"clientEventId":"01960000-0000-7000-8000-000000000012","occurredAt":"2026-11-18T00:01:00Z","sessionEnded":{"articleId":"11111111-1111-4111-8111-111111111111","surface":"READING_SURFACE_GALLERY","activeDwellMs":10000,"maxScrollRatio":1.0,"bodyLengthChars":500,"reachedEnd":true}}]}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "readArticleIds")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyQueueArticle(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading queue has capacity").
		UponReceiving("a QueueArticle unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/QueueArticle"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"articleId":     matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
				"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000021", uuidStringPattern),
				"occurredAt":    matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
				"surface":       matchers.String("READING_SURFACE_SWIPE"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"outcome": matchers.String("QUEUE_ARTICLE_OUTCOME_QUEUED"),
				"cap":     matchers.Integer(7),
				"items": matchers.EachLike(map[string]interface{}{
					"articleId":  matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
					"title":      matchers.Like("Queued Article"),
					"url":        matchers.Like("https://example.com/articles/1"),
					"sourceName": matchers.Like("example.com"),
					"queuedAt":   matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
					"expiresAt":  matchers.Regex("2026-11-20T00:00:00Z", rfc3339TimestampPattern),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/QueueArticle",
				strings.NewReader(`{"articleId":"11111111-1111-4111-8111-111111111111","clientEventId":"01960000-0000-7000-8000-000000000021","occurredAt":"2026-11-18T00:00:00Z","surface":"READING_SURFACE_SWIPE"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "QUEUE_ARTICLE_OUTCOME_QUEUED")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyQueueArticle_QueueFull(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading queue is at capacity").
		UponReceiving("a QueueArticle unary Connect-RPC request when the queue is full").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/QueueArticle"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"articleId":     matchers.Regex("88888888-8888-4888-8888-888888888888", uuidStringPattern),
				"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000022", uuidStringPattern),
				"occurredAt":    matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
				"surface":       matchers.String("READING_SURFACE_SWIPE"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"outcome": matchers.String("QUEUE_ARTICLE_OUTCOME_QUEUE_FULL"),
				"cap":     matchers.Integer(7),
				"items": matchers.EachLike(map[string]interface{}{
					"articleId": matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
					"title":     matchers.Like("Existing Queued Article"),
					"url":       matchers.Like("https://example.com/articles/1"),
					"queuedAt":  matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
					"expiresAt": matchers.Regex("2026-11-20T00:00:00Z", rfc3339TimestampPattern),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/QueueArticle",
				strings.NewReader(`{"articleId":"88888888-8888-4888-8888-888888888888","clientEventId":"01960000-0000-7000-8000-000000000022","occurredAt":"2026-11-18T00:00:00Z","surface":"READING_SURFACE_SWIPE"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "QUEUE_ARTICLE_OUTCOME_QUEUE_FULL")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyUnqueueArticle(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading service is available").
		UponReceiving("an UnqueueArticle unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/UnqueueArticle"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"articleId":     matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
				"clientEventId": matchers.Regex("01960000-0000-7000-8000-000000000023", uuidStringPattern),
				"occurredAt":    matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: map[string]interface{}{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/UnqueueArticle",
				strings.NewReader(`{"articleId":"11111111-1111-4111-8111-111111111111","clientEventId":"01960000-0000-7000-8000-000000000023","occurredAt":"2026-11-18T00:00:00Z"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyGetReadTodayQueue(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("reading queue has active items and read sources since midnight").
		UponReceiving("a GetReadTodayQueue unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.reading.v1.ReadingService/GetReadTodayQueue"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"readSince": matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"cap":            matchers.Integer(7),
				"readCountSince": matchers.Integer(2),
				"items": matchers.EachLike(map[string]interface{}{
					"articleId":  matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
					"title":      matchers.Like("Queued Article"),
					"url":        matchers.Like("https://example.com/articles/1"),
					"sourceName": matchers.Like("example.com"),
					"queuedAt":   matchers.Regex("2026-11-18T00:00:00Z", rfc3339TimestampPattern),
					"expiresAt":  matchers.Regex("2026-11-20T00:00:00Z", rfc3339TimestampPattern),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.reading.v1.ReadingService/GetReadTodayQueue",
				strings.NewReader(`{"readSince":"2026-11-18T00:00:00Z"}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "readCountSince")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxySetSubscriptionDailyCap(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("feed subscription exists for user").
		UponReceiving("a SetSubscriptionDailyCap unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.feeds.v2.FeedService/SetSubscriptionDailyCap"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(jwtHeaderExample, jwtHeaderPattern),
			},
			Body: matchers.MapMatcher{
				"feedLinkId": matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
				"dailyCap":   matchers.Integer(5),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"message":  matchers.Like("Daily cap updated"),
				"dailyCap": matchers.Integer(5),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.feeds.v2.FeedService/SetSubscriptionDailyCap",
				strings.NewReader(`{"feedLinkId":"11111111-1111-4111-8111-111111111111","dailyCap":5}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "dailyCap")
			return nil
		})
	require.NoError(t, err)
}

func TestBFFProxyGetFeedInflowStats(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("feed inflow stats are available for user").
		UponReceiving("a GetFeedInflowStats unary Connect-RPC request proxied by BFF").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.feeds.v2.FeedService/GetFeedInflowStats"),
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
			Body: matchers.MapMatcher{
				"windowDays": matchers.Integer(30),
				"stats": matchers.EachLike(map[string]interface{}{
					"feedLinkId":        matchers.Regex("11111111-1111-4111-8111-111111111111", uuidStringPattern),
					"url":               matchers.Like("https://example.com/feed.xml"),
					"dailyCap":          matchers.Integer(5),
					"inflowCount":       matchers.Like("120"),
					"readCount":         matchers.Like("18"),
					"openedCountLegacy": matchers.Like("4"),
					"readRate":          matchers.Like(0.15),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			backendURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			handler := createBFFHandler(backendURL)

			req := httptest.NewRequest(
				http.MethodPost,
				"/alt.feeds.v2.FeedService/GetFeedInflowStats",
				strings.NewReader(`{}`),
			)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alt-Backend-Token", createTestToken(t, "user"))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "inflowCount")
			return nil
		})
	require.NoError(t, err)
}
