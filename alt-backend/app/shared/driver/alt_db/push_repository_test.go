package alt_db

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// TestPushDeliveryBacklogAgeQueryCountsSendingRows pins the predicate rather
// than the plan.
//
// This is the reason the procedure exists. A row orphaned by a dispatcher that
// died mid-attempt stays in 'sending' until its lease expires; a query that
// matched only 'pending' would report an age of 0 while the queue was stuck,
// and the alert built on this metric would stay green through exactly the
// outage it was written to catch. Asserting on the SQL text is the only place
// that can be caught without a live database, because the wrong query returns
// a perfectly well-formed answer.
func TestPushDeliveryBacklogAgeQueryCountsSendingRows(t *testing.T) {
	require.Contains(t, pushDeliveryBacklogAgeQuery, "'sending'",
		"the backlog age must include rows a crashed dispatcher left in 'sending', "+
			"or a stuck queue reads as an empty one")
	require.Contains(t, pushDeliveryBacklogAgeQuery, "'pending'")

	// now() is the transaction start time, so inside a long transaction it
	// would understate the age by however long that transaction had been open.
	require.Contains(t, pushDeliveryBacklogAgeQuery, "clock_timestamp()")
	require.False(t, strings.Contains(pushDeliveryBacklogAgeQuery, " now()"),
		"the age must be read from clock_timestamp(), not the transaction clock")

	// An empty queue makes MIN() NULL; without COALESCE the scan fails and the
	// caller has a reason to skip the gauge instead of a value to publish.
	require.Contains(t, pushDeliveryBacklogAgeQuery, "COALESCE")
}

func TestPushRepository_PushDeliveryBacklogAge(t *testing.T) {
	tests := []struct {
		name        string
		seconds     float64
		pending     int64
		wantOldest  time.Duration
		wantPending int64
	}{
		{
			name:        "backlog reports age and depth",
			seconds:     123.5,
			pending:     7,
			wantOldest:  123500 * time.Millisecond,
			wantPending: 7,
		},
		{
			name:        "empty queue is zero rather than absent",
			seconds:     0,
			pending:     0,
			wantOldest:  0,
			wantPending: 0,
		},
		{
			// occurred_at is the producer's business time and nothing pins it
			// to the past, so a post-dated fact or a skewed producer clock
			// would otherwise publish a negative age.
			name:        "a future occurred_at clamps to zero",
			seconds:     -42,
			pending:     1,
			wantOldest:  0,
			wantPending: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			mock.ExpectQuery(regexp.QuoteMeta(pushDeliveryBacklogAgeQuery)).
				WillReturnRows(pgxmock.NewRows([]string{"oldest_pending_age_seconds", "pending_count"}).
					AddRow(tt.seconds, tt.pending))

			repo := &PushRepository{pool: mock}
			oldest, pending, err := repo.PushDeliveryBacklogAge(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantOldest, oldest)
			require.Equal(t, tt.wantPending, pending)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPushRepository_PushDeliveryBacklogAge_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(regexp.QuoteMeta(pushDeliveryBacklogAgeQuery)).
		WillReturnError(errors.New("db failed"))

	repo := &PushRepository{pool: mock}
	oldest, pending, err := repo.PushDeliveryBacklogAge(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "read push delivery backlog age")
	require.Zero(t, oldest)
	require.Zero(t, pending)
	require.NoError(t, mock.ExpectationsWereMet())
}
