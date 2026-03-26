package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockListDistinctUserIDsPort implements knowledge_home_port.ListDistinctUserIDsPort.
type mockListDistinctUserIDsPort struct {
	userIDs []uuid.UUID
	err     error
}

func (m *mockListDistinctUserIDsPort) ListDistinctUserIDs(_ context.Context) ([]uuid.UUID, error) {
	return m.userIDs, m.err
}

// mockListRecallSignalsByUserPort implements recall_signal_port.ListRecallSignalsByUserPort.
type mockListRecallSignalsByUserPort struct {
	signalsByUser map[uuid.UUID][]domain.RecallSignal
	err           error
}

func (m *mockListRecallSignalsByUserPort) ListRecallSignalsByUser(_ context.Context, userID uuid.UUID, _ int) ([]domain.RecallSignal, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.signalsByUser[userID], nil
}

// mockUpsertRecallCandidatePort implements recall_candidate_port.UpsertRecallCandidatePort.
type mockUpsertRecallCandidatePort struct {
	upserted []domain.RecallCandidate
	err      error
}

func (m *mockUpsertRecallCandidatePort) UpsertRecallCandidate(_ context.Context, candidate domain.RecallCandidate) error {
	if m.err != nil {
		return m.err
	}
	m.upserted = append(m.upserted, candidate)
	return nil
}

func TestProcessRecallSignals(t *testing.T) {
	logger.InitLogger()
	userID := uuid.New()

	t.Run("no users from port - no candidates", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: nil}
		signalPort := &mockListRecallSignalsByUserPort{}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("list users port error returns error", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{err: errors.New("db error")}
		signalPort := &mockListRecallSignalsByUserPort{}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list distinct user IDs")
	})

	t.Run("user with no signals - no candidates", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("SignalOpened older than 48h creates ReasonOpenedNotRevisited candidate", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		oldSignal := domain.RecallSignal{
			SignalID:   uuid.New(),
			UserID:     userID,
			ItemKey:    "article:old-item",
			SignalType: domain.SignalOpened,
			OccurredAt: time.Now().Add(-72 * time.Hour), // 72h ago
		}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: {oldSignal},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		assert.Equal(t, "article:old-item", candidatePort.upserted[0].ItemKey)
		assert.Equal(t, domain.ReasonOpenedNotRevisited, candidatePort.upserted[0].Reasons[0].Type)
		assert.InDelta(t, weightOpenedNotRevisited, candidatePort.upserted[0].RecallScore, 0.01)
	})

	t.Run("SignalOpened younger than 48h - below minRecallScore - no candidate", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		recentSignal := domain.RecallSignal{
			SignalID:   uuid.New(),
			UserID:     userID,
			ItemKey:    "article:recent-item",
			SignalType: domain.SignalOpened,
			OccurredAt: time.Now().Add(-24 * time.Hour), // 24h ago, < 48h
		}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: {recentSignal},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("SignalAugurReferenced creates ReasonRelatedToAugurQ candidate", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		signal := domain.RecallSignal{
			SignalID:   uuid.New(),
			UserID:     userID,
			ItemKey:    "article:augur-item",
			SignalType: domain.SignalAugurReferenced,
			OccurredAt: time.Now().Add(-1 * time.Hour),
		}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: {signal},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		assert.Equal(t, domain.ReasonRelatedToAugurQ, candidatePort.upserted[0].Reasons[0].Type)
		assert.InDelta(t, weightRelatedToAugur, candidatePort.upserted[0].RecallScore, 0.01)
	})

	t.Run("signal below minRecallScore is skipped", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		signal := domain.RecallSignal{
			SignalID:   uuid.New(),
			UserID:     userID,
			ItemKey:    "article:low-score",
			SignalType: domain.SignalTagInterest,
			OccurredAt: time.Now().Add(-1 * time.Hour),
		}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: {signal},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		// weightTagInterest = 0.15, which is below minRecallScore = 0.2
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("list signals error continues to next user", func(t *testing.T) {
		user2 := uuid.New()
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID, user2}}
		signalPort := &mockListRecallSignalsByUserPort{err: errors.New("db error")}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("upsert error continues to next item", func(t *testing.T) {
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID}}
		signals := []domain.RecallSignal{
			{
				SignalID:   uuid.New(),
				UserID:     userID,
				ItemKey:    "article:item1",
				SignalType: domain.SignalAugurReferenced,
				OccurredAt: time.Now().Add(-1 * time.Hour),
			},
			{
				SignalID:   uuid.New(),
				UserID:     userID,
				ItemKey:    "article:item2",
				SignalType: domain.SignalAugurReferenced,
				OccurredAt: time.Now().Add(-2 * time.Hour),
			},
		}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: signals,
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{err: errors.New("upsert failed")}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		// Both items attempted but both failed - no items in upserted
	})

	t.Run("processes multiple users from port", func(t *testing.T) {
		user2 := uuid.New()
		listUsersPort := &mockListDistinctUserIDsPort{userIDs: []uuid.UUID{userID, user2}}
		signalPort := &mockListRecallSignalsByUserPort{
			signalsByUser: map[uuid.UUID][]domain.RecallSignal{
				userID: {
					{
						SignalID: uuid.New(), UserID: userID,
						ItemKey: "article:u1", SignalType: domain.SignalAugurReferenced,
						OccurredAt: time.Now().Add(-1 * time.Hour),
					},
				},
				user2: {
					{
						SignalID: uuid.New(), UserID: user2,
						ItemKey: "article:u2", SignalType: domain.SignalAugurReferenced,
						OccurredAt: time.Now().Add(-1 * time.Hour),
					},
				},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := processRecallSignals(context.Background(), listUsersPort, signalPort, candidatePort, nil)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 2)
	})
}

func TestRecallDescriptionContainsTemporalContext(t *testing.T) {
	userID := uuid.New()

	t.Run("SignalOpened 3 days ago produces description with '3 days ago'", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:temporal-test", SignalType: domain.SignalOpened,
				OccurredAt: time.Now().Add(-72 * time.Hour), // 3 days ago
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		desc := candidatePort.upserted[0].Reasons[0].Description
		assert.Contains(t, desc, "3 days ago")
	})

	t.Run("SignalSearchRelated 5 hours ago produces description with '5 hours ago'", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:search-temporal", SignalType: domain.SignalSearchRelated,
				OccurredAt: time.Now().Add(-5 * time.Hour),
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		desc := candidatePort.upserted[0].Reasons[0].Description
		assert.Contains(t, desc, "5 hours ago")
	})

	t.Run("SignalSearchRelated with search_query in payload includes query in description", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:search-query", SignalType: domain.SignalSearchRelated,
				OccurredAt: time.Now().Add(-2 * time.Hour),
				Payload:    map[string]any{"search_query": "rust async"},
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		desc := candidatePort.upserted[0].Reasons[0].Description
		assert.Contains(t, desc, "rust async")
	})

	t.Run("SignalAugurReferenced produces description with temporal context", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:augur-temporal", SignalType: domain.SignalAugurReferenced,
				OccurredAt: time.Now().Add(-26 * time.Hour), // 1 day ago
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		desc := candidatePort.upserted[0].Reasons[0].Description
		assert.Contains(t, desc, "1 day ago")
	})

	t.Run("SignalRecapContextUnread produces description with temporal context", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:recap-temporal", SignalType: domain.SignalRecapContextUnread,
				OccurredAt: time.Now().Add(-96 * time.Hour), // 4 days ago
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		desc := candidatePort.upserted[0].Reasons[0].Description
		assert.Contains(t, desc, "4 days ago")
	})
}

func TestScoreRecallCandidates(t *testing.T) {
	userID := uuid.New()

	t.Run("empty signals - no candidates", func(t *testing.T) {
		candidatePort := &mockUpsertRecallCandidatePort{}
		err := ScoreRecallCandidates(context.Background(), userID, nil, candidatePort)
		require.NoError(t, err)
		assert.Empty(t, candidatePort.upserted)
	})

	t.Run("composite scoring from multiple signal types", func(t *testing.T) {
		signals := []domain.RecallSignal{
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:multi", SignalType: domain.SignalOpened,
				OccurredAt: time.Now().Add(-72 * time.Hour),
			},
			{
				SignalID: uuid.New(), UserID: userID,
				ItemKey: "article:multi", SignalType: domain.SignalAugurReferenced,
				OccurredAt: time.Now().Add(-1 * time.Hour),
			},
		}
		candidatePort := &mockUpsertRecallCandidatePort{}

		err := ScoreRecallCandidates(context.Background(), userID, signals, candidatePort)
		require.NoError(t, err)
		require.Len(t, candidatePort.upserted, 1)
		expectedScore := weightOpenedNotRevisited + weightRelatedToAugur
		assert.InDelta(t, expectedScore, candidatePort.upserted[0].RecallScore, 0.01)
		assert.Len(t, candidatePort.upserted[0].Reasons, 2)
	})
}
