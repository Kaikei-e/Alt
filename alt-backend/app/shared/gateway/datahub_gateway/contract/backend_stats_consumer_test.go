//go:build contract

// Pact CDC: alt-backend → alt-data-hub, dashboard statistics
// (ADR-000954 Wave 3 batch 5, capability catalog §2.M).
package contract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	statsFeedIDValue = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
)

// statsContext puts a signed-in user on the context.
//
// Every §2.M gateway method except FeedAmount resolves the tenant here and
// sends it as a field, so a test that called them with a bare context would
// exercise the refusal path rather than the contract.
//
// Email and ExpiresAt are set because domain.UserContext.IsValid() requires
// them — a user id alone reads as an expired session, which is the same
// refusal an absent user gets.
func statsContext(t *testing.T) context.Context {
	t.Helper()
	userID, err := parseTestUUID(statsUserIDValue)
	require.NoError(t, err)
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "reader@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// TestGetFeedAmountContract is the one count with no tenant. Its request body
// is empty, and that emptiness is the contract: a userId appearing here later
// would mean the number had quietly become per-user.
func TestGetFeedAmountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds").
		UponReceiving("a GetFeedAmount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetFeedAmount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(42)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.FeedAmount(context.Background())
			if countErr != nil {
				return fmt.Errorf("FeedAmount failed: %w", countErr)
			}
			assert.Equal(t, 42, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTotalArticlesCountContract pins the tenant field on the tenant-scoped
// counts.
//
// The driver read the user from the request context; over Connect the peer
// certificate names alt-backend and nothing about whose articles these are, so
// the field is the whole tenancy story. A provider that ignored it would answer
// every user with the same number and no test that mocked the gateway would
// notice.
func TestGetTotalArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the user").
		UponReceiving("a GetTotalArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetTotalArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(120)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.TotalArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("TotalArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 120, count)
			return nil
		})
	require.NoError(t, err)
}

func TestGetSummarizedArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has summarized articles for the user").
		UponReceiving("a GetSummarizedArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetSummarizedArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(80)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.SummarizedArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("SummarizedArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 80, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetUnsummarizedArticlesCountContract exists as its own interaction rather
// than being derived from the two counts above, because it is its own query: a
// summary can outlive the article it describes, so total minus summarized is
// not this number.
func TestGetUnsummarizedArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has unsummarized articles for the user").
		UponReceiving("a GetUnsummarizedArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetUnsummarizedArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(40)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.UnsummarizedArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("UnsummarizedArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 40, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTodayUnreadArticlesCountContract pins `since` on the request. "Today"
// is a wall-clock question and the provider has no timezone for it, so the
// bound travels rather than being derived on the far side.
func TestGetTodayUnreadArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	since := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has unread feeds for the user since the bound").
		UponReceiving("a GetTodayUnreadArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetTodayUnreadArticlesCount"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": statsUserIDValue,
				"since":  "2026-07-31T00:00:00Z",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(7)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.TodayUnreadArticlesCount(statsContext(t), since)
			if countErr != nil {
				return fmt.Errorf("TodayUnreadArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 7, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTrendStatsContract pins the window enum and the granularity that comes
// back with it.
//
// The two are not independent: a 7-day window is bucketed daily because that is
// what the query's date_trunc does, and the caller labels its axis from the
// answer rather than from its own request. Sending the window as an enum is
// what keeps "90d" from being a runtime error on a value the contract had
// implied was acceptable.
func TestGetTrendStatsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has trend data for the user").
		UponReceiving("a GetTrendStats request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetTrendStats"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": statsUserIDValue,
				"window": "TREND_WINDOW_7D",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"points": matchers.EachLike(map[string]interface{}{
					"bucket":       matchers.Like("2026-07-30T00:00:00Z"),
					"articles":     matchers.Like(12),
					"summarized":   matchers.Like(9),
					"feedActivity": matchers.Like(3),
				}, 1),
				"granularity": matchers.Like("TREND_GRANULARITY_DAILY"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			series, statsErr := gw.TrendStats(statsContext(t), "7d")
			if statsErr != nil {
				return fmt.Errorf("TrendStats failed: %w", statsErr)
			}
			require.Len(t, series.Points, 1)
			assert.Equal(t, 12, series.Points[0].Articles)
			assert.Equal(t, "daily", series.Granularity)
			return nil
		})
	require.NoError(t, err)
}

func TestListUserFeedIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has read state for the user").
		UponReceiving("a ListUserFeedIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListUserFeedIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedIds": matchers.EachLike(matchers.Regex(statsFeedIDValue, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			ids, listErr := gw.UserFeedIDs(statsContext(t))
			if listErr != nil {
				return fmt.Errorf("UserFeedIDs failed: %w", listErr)
			}
			require.Len(t, ids, 1)
			assert.Equal(t, statsFeedIDValue, ids[0].String())
			return nil
		})
	require.NoError(t, err)
}
