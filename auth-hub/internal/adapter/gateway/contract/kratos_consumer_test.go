//go:build contract

// Package contract contains Consumer-Driven Contract tests for auth-hub → Kratos.
//
// These tests verify that auth-hub's expectations of the Ory Kratos
// /sessions/whoami API are documented as Pact contracts.
// Kratos is an external service, so only consumer tests are written (no provider verification).
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-hub/internal/adapter/gateway"
	"auth-hub/internal/domain"
)

const pactDir = "../../../../../pacts"

func newKratosPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "auth-hub",
		Provider: "kratos",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func TestKratosValidateSession_ValidCookie(t *testing.T) {
	mockProvider := newKratosPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a valid session exists for user-001").
		UponReceiving("a ToSession request with a valid session cookie").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/sessions/whoami"),
			Headers: matchers.MapMatcher{
				"Cookie": matchers.String("ory_kratos_session=valid-session-token"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"id":     "session-abc-123",
				"active": true,
				"identity": map[string]interface{}{
					"id":         "user-001",
					"schema_id":  "default",
					"schema_url": "http://kratos:4433/schemas/default",
					"traits": map[string]interface{}{
						"email": "user@example.com",
						"role":  "user",
					},
					"created_at": "2026-01-01T00:00:00Z",
				},
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			kratosURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			gw := gateway.NewKratosGateway(kratosURL, "", 5*time.Second)

			identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=valid-session-token")
			if err != nil {
				return fmt.Errorf("ValidateSession failed: %w", err)
			}

			assert.Equal(t, "user-001", identity.UserID)
			assert.Equal(t, "user@example.com", identity.Email)
			assert.Equal(t, "user", identity.Role)
			assert.Equal(t, "session-abc-123", identity.SessionID)
			return nil
		})
	require.NoError(t, err)
}

func TestKratosValidateSession_InvalidCookie(t *testing.T) {
	mockProvider := newKratosPact(t)

	err := mockProvider.
		AddInteraction().
		Given("no valid session exists").
		UponReceiving("a ToSession request with an invalid session cookie").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/sessions/whoami"),
			Headers: matchers.MapMatcher{
				"Cookie": matchers.String("ory_kratos_session=expired-token"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 401,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    401,
					"status":  "Unauthorized",
					"message": "No active session was found in this request",
				},
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			kratosURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			gw := gateway.NewKratosGateway(kratosURL, "", 5*time.Second)

			identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=expired-token")
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, domain.ErrAuthFailed)
			return nil
		})
	require.NoError(t, err)
}

func TestKratosValidateSession_RateLimited(t *testing.T) {
	mockProvider := newKratosPact(t)

	err := mockProvider.
		AddInteraction().
		Given("rate limit is exceeded").
		UponReceiving("a ToSession request when rate limited").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/sessions/whoami"),
			Headers: matchers.MapMatcher{
				"Cookie": matchers.String("ory_kratos_session=any-token"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 429,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.Like(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    429,
					"status":  "Too Many Requests",
					"message": "API rate limit exceeded",
				},
			}),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			kratosURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			gw := gateway.NewKratosGateway(kratosURL, "", 5*time.Second)

			identity, err := gw.ValidateSession(context.Background(), "ory_kratos_session=any-token")
			assert.Nil(t, identity)
			assert.ErrorIs(t, err, domain.ErrRateLimited)
			return nil
		})
	require.NoError(t, err)
}
