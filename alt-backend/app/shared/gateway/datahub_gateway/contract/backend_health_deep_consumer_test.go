//go:build contract

package contract

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"alt/shared/driver/datahub_client"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/require"
)

// Deep health is the mTLS data-path probe cmd/backend's ops /health/deep
// runs via datahub_client.Ping. It is not a DataHubService RPC: cheap
// /health is liveness, this envelope is whether the owned DB/pool answered.
//
// What these interactions pin:
//
//   - GET /health/deep on the data-plane listener, not ops :9110 and not
//     cheap /health.
//   - The envelope status vocabulary is pass | warn | fail. Ping accepts
//     pass and warn; fail is unavailable.
//   - latency_ms and cached are omitted on purpose. Those change every run
//     and are not what the consumer branches on.

func TestPingDeepHealthPassContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub database is reachable").
		UponReceiving("a GET /health/deep from alt-backend when the data path is up").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/health/deep"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"status":  matchers.String("pass"),
				"service": matchers.Like("alt-data-hub"),
				"checks": matchers.EachLike(map[string]interface{}{
					"name":     matchers.String("database"),
					"status":   matchers.String("pass"),
					"critical": matchers.Like(true),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			if err := datahub_client.Ping(context.Background(), http.DefaultClient, dataHubBaseURL(config)); err != nil {
				return fmt.Errorf("Ping rejected a pass envelope: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestPingDeepHealthWarnContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub data path is degraded").
		UponReceiving("a GET /health/deep from alt-backend when a non-critical check failed").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/health/deep"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"status":  matchers.String("warn"),
				"service": matchers.Like("alt-data-hub"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			if err := datahub_client.Ping(context.Background(), http.DefaultClient, dataHubBaseURL(config)); err != nil {
				return fmt.Errorf("Ping rejected a warn envelope: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestPingDeepHealthFailContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub database is unavailable").
		UponReceiving("a GET /health/deep from alt-backend when the owned DB is down").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/health/deep"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  503,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"status":  matchers.String("fail"),
				"service": matchers.Like("alt-data-hub"),
				"checks": matchers.EachLike(map[string]interface{}{
					"name":     matchers.String("database"),
					"status":   matchers.String("fail"),
					"critical": matchers.Like(true),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			err := datahub_client.Ping(context.Background(), http.DefaultClient, dataHubBaseURL(config))
			if err == nil {
				return fmt.Errorf("Ping accepted a fail envelope")
			}
			return nil
		})
	require.NoError(t, err)
}

func dataHubBaseURL(config consumer.MockServerConfig) string {
	return fmt.Sprintf("http://%s:%d", config.Host, config.Port)
}
