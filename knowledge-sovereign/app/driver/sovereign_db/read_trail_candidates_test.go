package sovereign_db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeriveTrailContinuationCandidates_DismissalsAreNotContact pins the two
// halves of the continuation truthfulness gate: a dismissal must not count as
// contact with a thread (it is the opposite), and an item the user dismissed
// from Home must not be proposed back to them. The sibling cluster query has
// carried the dismissed_at gate all along; its absence here is what put 386 of
// 391 continuation branches on dismissed articles.
func TestDeriveTrailContinuationCandidates_DismissalsAreNotContact(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.DeriveTrailContinuationCandidates(context.Background(), uuid.New(), 1)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one continuation-candidates query")

	call := mock.queryCalls[0]
	assert.Contains(t, call.SQL, "verb = ANY($2::text[])",
		"only engagement verbs count as contact with a thread")
	assert.Contains(t, call.SQL, "khi.dismissed_at IS NULL",
		"a dismissed item must never be proposed back to the user")
	assert.Equal(t, EngagementVerbs, call.Args[1])
}

// TestDeriveTrailContinuationCandidates_PinsStaleWindowNotDeepAndAlreadyProposed
// pins the Wave 11 (D27/D28) Continuation derivation shape: only items with
// 1-3 raw contacts, no 'asked' verb, and no engaged act-outcome ("not deep")
// qualify; the last contact must sit inside the stale/expire window (quiet,
// not cold, not still being actively read); items that already carry a
// continuation branch (open or resolved) are excluded so a taken or dismissed
// proposal is never re-proposed; and the result is capped server-side,
// most-recent-contact first.
func TestDeriveTrailContinuationCandidates_PinsStaleWindowNotDeepAndAlreadyProposed(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	before := time.Now()
	_, err := repo.DeriveTrailContinuationCandidates(context.Background(), userID, 1)
	after := time.Now()
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one continuation-candidates query")

	call := mock.queryCalls[0]
	sql := call.SQL
	assert.Contains(t, sql, "contact_count BETWEEN 1 AND 3", "not-deep gate: 1-3 raw contacts, not 0 and not 4+")
	assert.Contains(t, sql, "NOT ic.has_ask", "an asked verb already reads as deep — not continuation material")
	assert.Contains(t, sql, "NOT COALESCE(ie.engaged, FALSE)", "an engaged act-outcome already reads as deep")
	assert.Contains(t, sql, "ic.last_contact_at <= $4", "must be older than the stale-after cutoff")
	assert.Contains(t, sql, "ic.last_contact_at >= $5", "must be newer than the expire-after cutoff (quiet, not cold)")
	assert.Contains(t, sql, "coalesce(khi.title, '') <> ''", "title must be resolvable — no unnameable proposals")
	assert.Contains(t, sql, "relation_kind = 'continuation'", "excludes items that already carry a continuation branch")
	assert.Contains(t, sql, "NOT EXISTS", "already-proposed continuation branches must not be re-proposed")
	assert.Contains(t, sql, "ORDER BY ic.last_contact_at DESC", "most recent last-contact first")
	assert.Contains(t, sql, "LIMIT $6")

	require.Len(t, call.Args, 6)
	assert.Equal(t, userID, call.Args[0])
	assert.Equal(t, EngagementVerbs, call.Args[1])
	assert.Equal(t, EngagedDwellMs, call.Args[2])

	staleCutoff, ok := call.Args[3].(time.Time)
	require.True(t, ok, "arg[3] must be the stale-after cutoff timestamp")
	expireCutoff, ok := call.Args[4].(time.Time)
	require.True(t, ok, "arg[4] must be the expire-after cutoff timestamp")

	assert.WithinDuration(t, before.Add(-continuationStaleAfter), staleCutoff, after.Sub(before)+time.Second)
	assert.WithinDuration(t, before.Add(-continuationExpireAfter), expireCutoff, after.Sub(before)+time.Second)
	assert.Equal(t, 1, call.Args[5])
}

func TestCalcTrailContinuationCutoffs(t *testing.T) {
	tests := []struct {
		name       string
		ref        time.Time
		wantStale  time.Time
		wantExpire time.Time
	}{
		{
			name:       "fixed calendar date",
			ref:        time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC),
			wantStale:  time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC),
			wantExpire: time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC),
		},
		{
			name:       "fixed unix timestamp",
			ref:        time.Unix(1_700_000_000, 0).UTC(),
			wantStale:  time.Unix(1_700_000_000, 0).UTC().Add(-continuationStaleAfter),
			wantExpire: time.Unix(1_700_000_000, 0).UTC().Add(-continuationExpireAfter),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStale, gotExpire := calcTrailContinuationCutoffs(tc.ref)
			assert.Equal(t, tc.wantStale, gotStale)
			assert.Equal(t, tc.wantExpire, gotExpire)
		})
	}
}

func TestScanTrailClusterCandidateRow(t *testing.T) {
	tests := []struct {
		name     string
		itemKey  string
		title    string
		tags     []string
		scanErr  error
		wantErr  bool
		wantCand TrailClusterCandidate
	}{
		{
			name:    "valid candidate",
			itemKey: "item-10",
			title:   "Title 10",
			tags:    []string{"go", "clean-architecture"},
			wantCand: TrailClusterCandidate{
				TargetItemKey: "item-10",
				TargetTitle:   "Title 10",
				SharedTags:    []string{"go", "clean-architecture"},
			},
		},
		{
			name:    "scan error",
			scanErr: errors.New("cluster scan failed"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRow{
				scanFunc: func(dest ...interface{}) error {
					if tc.scanErr != nil {
						return tc.scanErr
					}
					*dest[0].(*string) = tc.itemKey
					*dest[1].(*string) = tc.title
					*dest[2].(*[]string) = tc.tags
					return nil
				},
			}

			c, err := scanTrailClusterCandidateRow(mock)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantCand, c)
		})
	}
}

func TestScanTrailContinuationCandidateRow(t *testing.T) {
	refTime := time.Date(2026, time.September, 20, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		itemKey  string
		title    string
		lastAt   time.Time
		verb     string
		scanErr  error
		wantErr  bool
		wantCand TrailContinuationCandidate
	}{
		{
			name:    "valid candidate",
			itemKey: "item-20",
			title:   "Title 20",
			lastAt:  refTime,
			verb:    "read",
			wantCand: TrailContinuationCandidate{
				TargetItemKey: "item-20",
				TargetTitle:   "Title 20",
				LastContactAt: refTime,
				Verb:          "read",
			},
		},
		{
			name:    "scan error",
			scanErr: errors.New("continuation scan failed"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRow{
				scanFunc: func(dest ...interface{}) error {
					if tc.scanErr != nil {
						return tc.scanErr
					}
					*dest[0].(*string) = tc.itemKey
					*dest[1].(*string) = tc.title
					*dest[2].(*time.Time) = tc.lastAt
					*dest[3].(*string) = tc.verb
					return nil
				},
			}

			c, err := scanTrailContinuationCandidateRow(mock)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantCand, c)
		})
	}
}
