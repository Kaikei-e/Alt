package alt_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_AppendRecallSignal_PassesJSONAsTextForPgBouncerCompat(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	signalID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	// pgxmock verifies the argument type: string means text protocol (PgBouncer-safe),
	// []byte would mean bytea protocol (causes "invalid input syntax for type json").
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`)).
		WithArgs(
			signalID,
			userID,
			"article:abc123",
			domain.SignalOpened,
			1.0,
			occurredAt,
			`{"action_type":"open","source":"home_action"}`,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.AppendRecallSignal(context.Background(), domain.RecallSignal{
		SignalID:       signalID,
		UserID:         userID,
		ItemKey:        "article:abc123",
		SignalType:     domain.SignalOpened,
		SignalStrength: 1.0,
		OccurredAt:     occurredAt,
		Payload:        map[string]any{"source": "home_action", "action_type": "open"},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_AppendRecallSignal_NilPayloadDefaultsToEmptyObject(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	signalID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`)).
		WithArgs(
			signalID,
			userID,
			"article:abc123",
			domain.SignalOpened,
			1.0,
			occurredAt,
			`{}`,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.AppendRecallSignal(context.Background(), domain.RecallSignal{
		SignalID:       signalID,
		UserID:         userID,
		ItemKey:        "article:abc123",
		SignalType:     domain.SignalOpened,
		SignalStrength: 1.0,
		OccurredAt:     occurredAt,
		Payload:        nil, // nil should become "{}"
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
