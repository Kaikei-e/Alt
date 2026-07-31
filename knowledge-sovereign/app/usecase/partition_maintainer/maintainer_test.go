package partition_maintainer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ensureCall struct {
	Table      string
	StartMonth time.Time
	Months     int
}

type fakePartitionRepo struct {
	calls   []ensureCall
	created map[string][]string
	errs    map[string]error
}

func (f *fakePartitionRepo) EnsurePartitions(_ context.Context, table string, startMonth time.Time, months int) ([]string, error) {
	f.calls = append(f.calls, ensureCall{Table: table, StartMonth: startMonth, Months: months})
	return f.created[table], f.errs[table]
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestMaintainer_RunOnce(t *testing.T) {
	t.Run("asks for the current month plus the lookahead, per table", func(t *testing.T) {
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{
			Tables:      []string{"knowledge_events", "knowledge_user_events"},
			MonthsAhead: 3,
			Clock:       fixedClock(time.Date(2026, 7, 29, 13, 4, 5, 0, time.UTC)),
		})

		require.NoError(t, m.RunOnce(context.Background()))
		assert.Equal(t, []ensureCall{
			{Table: "knowledge_events", StartMonth: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Months: 4},
			{Table: "knowledge_user_events", StartMonth: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Months: 4},
		}, repo.calls)
	})

	t.Run("defaults cover both partitioned event tables", func(t *testing.T) {
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{Clock: fixedClock(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))})

		require.NoError(t, m.RunOnce(context.Background()))
		require.Len(t, repo.calls, 2)
		assert.Equal(t, "knowledge_events", repo.calls[0].Table)
		assert.Equal(t, "knowledge_user_events", repo.calls[1].Table)
		assert.Equal(t, defaultMonthsAhead+1, repo.calls[0].Months)
	})

	t.Run("normalizes a clock late in a 31-day month to the first", func(t *testing.T) {
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{
			Tables: []string{"knowledge_events"},
			Clock:  fixedClock(time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)),
		})

		require.NoError(t, m.RunOnce(context.Background()))
		require.Len(t, repo.calls, 1)
		assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), repo.calls[0].StartMonth)
	})

	t.Run("rolls the lookahead over the year boundary", func(t *testing.T) {
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{
			Tables:      []string{"knowledge_events"},
			MonthsAhead: 3,
			Clock:       fixedClock(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)),
		})

		require.NoError(t, m.RunOnce(context.Background()))
		require.Len(t, repo.calls, 1)
		// December + 3 ahead = Dec, Jan, Feb, Mar — the driver turns this into
		// y2026m12 .. y2027m03.
		assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), repo.calls[0].StartMonth)
		assert.Equal(t, 4, repo.calls[0].Months)
	})

	t.Run("a non-UTC clock still resolves the UTC month", func(t *testing.T) {
		// occurred_at is TIMESTAMPTZ and the partition bounds are UTC dates,
		// so a JST-local clock at 2026-08-01 08:00 must still ensure the
		// UTC month it falls in (2026-07-31 23:00Z → July).
		jst := time.FixedZone("JST", 9*3600)
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{
			Tables: []string{"knowledge_events"},
			Clock:  fixedClock(time.Date(2026, 8, 1, 8, 0, 0, 0, jst)),
		})

		require.NoError(t, m.RunOnce(context.Background()))
		require.Len(t, repo.calls, 1)
		assert.Equal(t, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), repo.calls[0].StartMonth)
	})

	t.Run("one failing table does not skip the others", func(t *testing.T) {
		repo := &fakePartitionRepo{
			errs:    map[string]error{"knowledge_events": errors.New("boom")},
			created: map[string][]string{"knowledge_user_events": {"knowledge_user_events_y2026m08"}},
		}
		m := New(repo, nil, Config{
			Tables: []string{"knowledge_events", "knowledge_user_events"},
			Clock:  fixedClock(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)),
		})

		err := m.RunOnce(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "knowledge_events")
		require.Len(t, repo.calls, 2, "the second table must still be ensured")
	})

	t.Run("is idempotent: a run that creates nothing is not an error", func(t *testing.T) {
		repo := &fakePartitionRepo{}
		m := New(repo, nil, Config{
			Tables: []string{"knowledge_events"},
			Clock:  fixedClock(time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)),
		})

		require.NoError(t, m.RunOnce(context.Background()))
		require.NoError(t, m.RunOnce(context.Background()))
		assert.Equal(t, repo.calls[0], repo.calls[1], "both runs ask for the same window")
	})
}
