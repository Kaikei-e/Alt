package service

import (
	"context"
	"log/slog"
	"testing"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// todaysUsageAt builds a persisted api_usage_tracking row for the current day so
// ShouldResetUsage() stays false and the test exercises the increment path.
func todaysUsageAt(zone1 int) *models.APIUsageTracking {
	usage := models.NewAPIUsageTracking()
	usage.Zone1Requests = zone1
	return usage
}

// A successful Inoreader call must land in api_usage_tracking. Without that write
// the 100-req/day Zone 1 budget is only ever guessed at by the 16m scheduler.
func TestFetchSubscriptions_PersistsZone1UsageToRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchSubscriptionList(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseSubscriptionsResponse(gomock.Any()).
		Return([]*models.Subscription{}, nil)

	usageRepo := mocks.NewMockAPIUsageRepository(ctrl)
	usageRepo.EXPECT().
		GetTodaysUsage(gomock.Any()).
		Return(todaysUsageAt(41), nil)

	var persisted *models.APIUsageTracking
	usageRepo.EXPECT().
		UpdateUsageRecord(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, usage *models.APIUsageTracking) error {
			persisted = usage
			return nil
		})

	svc := NewInoreaderService(client, usageRepo, stubTokenProvider{}, slog.Default())

	_, err := svc.FetchSubscriptions(context.Background())
	require.NoError(t, err)

	require.NotNil(t, persisted, "every Inoreader call must be written to api_usage_tracking")
	assert.Equal(t, 42, persisted.Zone1Requests,
		"the read-only /subscription/list call must advance the Zone 1 counter")
}

// The persisted daily counter — not the process-local zero value — is what the
// quota guard has to gate on, so a sidecar that starts at 89/100 gets exactly one
// call before CheckAPIRateLimit refuses the rest of the day.
func TestFetchSubscriptions_BlocksOnceZone1BudgetIsExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchSubscriptionList(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, nil).
		Times(1)
	client.EXPECT().
		ParseSubscriptionsResponse(gomock.Any()).
		Return([]*models.Subscription{}, nil).
		Times(1)

	usageRepo := mocks.NewMockAPIUsageRepository(ctrl)
	usageRepo.EXPECT().
		GetTodaysUsage(gomock.Any()).
		Return(todaysUsageAt(89), nil).
		Times(1)
	usageRepo.EXPECT().
		UpdateUsageRecord(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	svc := NewInoreaderService(client, usageRepo, stubTokenProvider{}, slog.Default())
	ctx := context.Background()

	_, err := svc.FetchSubscriptions(ctx)
	require.NoError(t, err, "the 90th request is still inside the safety buffer")

	allowed, remaining := svc.CheckAPIRateLimit()
	assert.False(t, allowed, "90/100 with a 10 request safety buffer must stop further calls")
	assert.Equal(t, 0, remaining)

	_, err = svc.FetchSubscriptions(ctx)
	require.Error(t, err, "the exhausted Zone 1 budget must reject the next call")
	assert.ErrorContains(t, err, "API rate limit exceeded")
}

// An unwired usage repository must not hand the fetch loop an unlimited budget:
// the in-process counter still advances so the guard keeps working, degraded.
func TestFetchSubscriptions_UnwiredUsageRepositoryStillAdvancesGuard(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	client := mocks.NewMockInoreaderClient(ctrl)
	client.EXPECT().
		FetchSubscriptionList(gomock.Any(), gomock.Any()).
		Return(map[string]interface{}{}, nil)
	client.EXPECT().
		ParseSubscriptionsResponse(gomock.Any()).
		Return([]*models.Subscription{}, nil)

	svc := NewInoreaderService(client, nil, stubTokenProvider{}, slog.Default())

	_, err := svc.FetchSubscriptions(context.Background())
	require.NoError(t, err)

	_, remaining := svc.CheckAPIRateLimit()
	assert.Equal(t, 89, remaining, "one call must be subtracted from the 90 request safe budget")
}
