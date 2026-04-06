package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRecapPortForDigest implements recap_port.RecapPort for digest availability tests.
type mockRecapPortForDigest struct {
	sevenDayRecap   *domain.RecapSummary
	sevenDayErr     error
	eveningPulse    *domain.EveningPulse
	eveningPulseErr error
}

func (m *mockRecapPortForDigest) GetSevenDayRecap(_ context.Context) (*domain.RecapSummary, error) {
	return m.sevenDayRecap, m.sevenDayErr
}

func (m *mockRecapPortForDigest) GetThreeDayRecap(_ context.Context) (*domain.RecapSummary, error) {
	return nil, nil
}

func (m *mockRecapPortForDigest) GetEveningPulse(_ context.Context, _ string) (*domain.EveningPulse, error) {
	return m.eveningPulse, m.eveningPulseErr
}

func (m *mockRecapPortForDigest) SearchRecapsByTag(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, error) {
	return nil, nil
}

func (m *mockRecapPortForDigest) SearchRecapsByQuery(_ context.Context, _ string, _ int) ([]*domain.RecapSearchResult, error) {
	return nil, nil
}

// mockDigestUpsertPort implements today_digest_port.UpsertTodayDigestPort for testing.
type mockDigestUpsertPort struct {
	upserted []domain.TodayDigest
	err      error
}

func (m *mockDigestUpsertPort) UpsertTodayDigest(_ context.Context, digest domain.TodayDigest) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, digest)
	return nil
}

func TestDigestAvailabilityReconcile(t *testing.T) {
	logger.InitLogger()
	userID := uuid.New()

	t.Run("recap available - sets WeeklyRecapAvailable true", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayRecap:   &domain.RecapSummary{JobID: "job-1"},
			eveningPulseErr: domain.ErrEveningPulseNotFound,
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.True(t, digestPort.upserted[0].WeeklyRecapAvailable)
		assert.False(t, digestPort.upserted[0].EveningPulseAvailable)
		assert.Equal(t, userID, digestPort.upserted[0].UserID)
	})

	t.Run("recap ErrRecapNotFound - sets WeeklyRecapAvailable false", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr:     domain.ErrRecapNotFound,
			eveningPulseErr: domain.ErrEveningPulseNotFound,
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.False(t, digestPort.upserted[0].WeeklyRecapAvailable)
	})

	t.Run("both transient errors - UpsertTodayDigest not called", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr:     errors.New("connection refused"),
			eveningPulseErr: errors.New("timeout"),
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		assert.Empty(t, digestPort.upserted)
	})

	t.Run("pulse PulseStatusError - sets EveningPulseAvailable false", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr: domain.ErrRecapNotFound,
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusError,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.False(t, digestPort.upserted[0].EveningPulseAvailable)
	})

	t.Run("pulse PulseStatusNormal - sets EveningPulseAvailable true", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr: domain.ErrRecapNotFound,
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusNormal,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.True(t, digestPort.upserted[0].EveningPulseAvailable)
	})

	t.Run("pulse PulseStatusQuietDay - sets EveningPulseAvailable true", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr: domain.ErrRecapNotFound,
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusQuietDay,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.True(t, digestPort.upserted[0].EveningPulseAvailable)
	})

	t.Run("pulse PulseStatusPartial - sets EveningPulseAvailable true", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr: domain.ErrRecapNotFound,
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusPartial,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		assert.True(t, digestPort.upserted[0].EveningPulseAvailable)
	})

	t.Run("no users from port - no-op", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: nil}
		recapPort := &mockRecapPortForDigest{}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		assert.Empty(t, digestPort.upserted)
	})

	t.Run("list users port error returns error", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{err: errors.New("db error")}
		recapPort := &mockRecapPortForDigest{}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list distinct user IDs")
	})

	t.Run("multiple users - upserts for each", func(t *testing.T) {
		user2 := uuid.New()
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID, user2}}
		recapPort := &mockRecapPortForDigest{
			sevenDayRecap: &domain.RecapSummary{JobID: "job-1"},
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusNormal,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 2)
		assert.True(t, digestPort.upserted[0].WeeklyRecapAvailable)
		assert.True(t, digestPort.upserted[0].EveningPulseAvailable)
		assert.True(t, digestPort.upserted[1].WeeklyRecapAvailable)
		assert.True(t, digestPort.upserted[1].EveningPulseAvailable)
	})

	t.Run("recap transient error but pulse available - partial reconcile", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recapPort := &mockRecapPortForDigest{
			sevenDayErr: errors.New("connection refused"),
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusNormal,
			},
		}
		digestPort := &mockDigestUpsertPort{}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, digestPort)
		require.NoError(t, err)
		require.Len(t, digestPort.upserted, 1)
		// recap skipped → false (OR merge preserves existing true in DB)
		assert.False(t, digestPort.upserted[0].WeeklyRecapAvailable)
		// pulse succeeded → true
		assert.True(t, digestPort.upserted[0].EveningPulseAvailable)
	})

	t.Run("upsert error for one user - continues to next", func(t *testing.T) {
		user2 := uuid.New()
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID, user2}}
		recapPort := &mockRecapPortForDigest{
			sevenDayRecap: &domain.RecapSummary{JobID: "job-1"},
			eveningPulse: &domain.EveningPulse{
				Status: domain.PulseStatusNormal,
			},
		}
		// Use a custom mock that fails on first call
		customPort := &mockDigestUpsertPortWithCounter{failOnCall: 1}

		err := digestAvailabilityReconcile(context.Background(), listUsersPort, recapPort, customPort)
		require.NoError(t, err)
		// First user failed, second user succeeded
		require.Len(t, customPort.upserted, 1)
		assert.Equal(t, user2, customPort.upserted[0].UserID)
	})
}

// mockDigestUpsertPortWithCounter fails on a specific call number (1-indexed).
type mockDigestUpsertPortWithCounter struct {
	upserted   []domain.TodayDigest
	failOnCall int
	callCount  int
}

func (m *mockDigestUpsertPortWithCounter) UpsertTodayDigest(_ context.Context, digest domain.TodayDigest) error {
	m.callCount++
	if m.callCount == m.failOnCall {
		return errors.New("upsert failed")
	}
	m.upserted = append(m.upserted, digest)
	return nil
}
