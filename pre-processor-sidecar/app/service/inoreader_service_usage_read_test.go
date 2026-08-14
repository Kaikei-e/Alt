package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"pre-processor-sidecar/mocks"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A transient api_usage_tracking read failure must never be mistaken for "no row
// for today". UpdateUsageRecord overwrites the day's counters wholesale, so a
// single connection reset would rewrite a day sitting at 61/100 down to 1 — and
// the in-process counter hides it until the next restart re-inherits the 1 and
// burns through the real 100 req/day Inoreader budget.
func TestFetchSubscriptions_TransientUsageReadErrorLeavesPersistedCounterAlone(t *testing.T) {
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
		Return(nil, errors.New("read tcp 10.0.0.5:5432: connection reset by peer"))
	// No CreateUsageRecord / UpdateUsageRecord expectation on purpose: writing
	// today's row from a read we never got is the corruption under test, and
	// gomock fails the test if either is called.

	svc := NewInoreaderService(client, usageRepo, stubTokenProvider{}, slog.Default())

	_, err := svc.FetchSubscriptions(context.Background())
	require.NoError(t, err, "a usage tracking write failure must not fail the fetch itself")

	// The guard still advances in-process, so an unreachable Postgres cannot read
	// as an unlimited budget.
	_, remaining := svc.CheckAPIRateLimit()
	assert.Equal(t, 89, remaining, "one call must be subtracted from the 90 request safe budget")
}

// The first call of the day genuinely has no row yet. That case — and only that
// case — may seed a fresh counter.
func TestFetchSubscriptions_MissingTodaysRowSeedsFreshCounter(t *testing.T) {
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
		Return(nil, repository.ErrNoUsageRecordToday)
	usageRepo.EXPECT().
		CreateUsageRecord(gomock.Any(), gomock.Any()).
		Return(nil)

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

	require.NotNil(t, persisted, "the day's first call must still be written to api_usage_tracking")
	assert.Equal(t, 1, persisted.Zone1Requests)
}
