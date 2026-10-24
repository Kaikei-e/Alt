package sovereign_db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type digestRecallTestRows struct {
	fakeEmptyRows
	scans []func(dest ...any) error
	idx   int
	err   error
}

func (r *digestRecallTestRows) Next() bool {
	if r.idx < len(r.scans) {
		r.idx++
		return true
	}
	return false
}

func (r *digestRecallTestRows) Scan(dest ...any) error {
	return r.scans[r.idx-1](dest...)
}

func (r *digestRecallTestRows) Err() error { return r.err }

// TestGetRecallCandidates_ResurfacesAfterSnoozeExpires pins the fix for the
// permanent-snooze bug: the original filter excluded any row where
// snoozed_until IS NULL, which after a snooze is set means the candidate
// never resurfaces even once snoozed_until has passed (snooze became a
// de-facto permanent dismiss). The fix allows resurfacing once the snooze
// window has elapsed.
func TestGetRecallCandidates_ResurfacesAfterSnoozeExpires(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetRecallCandidates(context.Background(), uuid.New(), 10)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	assert.Contains(t, sql, "(rcv.snoozed_until IS NULL OR rcv.snoozed_until <= now())",
		"snooze filter must allow candidates to resurface once snoozed_until has passed")

	selectList, _, found := strings.Cut(sql, "FROM recall_candidate_view")
	require.True(t, found)
	assert.Contains(t, selectList, "COALESCE(khi.tenant_id, '00000000-0000-0000-0000-000000000000'::uuid)")
	assert.Contains(t, selectList, "COALESCE(khi.projection_version, 0)")
}

func TestGetRecallCandidates_PopulatesEmbeddedItemTenantAndProjectionVersion(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	refID := uuid.New()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	title := "Recalled Article"
	summary := "Summary text"
	score := 0.88
	summaryState := "ready"
	url := "https://example.com/1"
	itemType := "article"

	mock := &mockPgx{
		queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
			return &digestRecallTestRows{
				scans: []func(dest ...any) error{
					func(dest ...any) error {
						*dest[0].(*uuid.UUID) = userID
						*dest[1].(*string) = "article:1"
						*dest[2].(*float64) = 0.91
						*dest[3].(*[]byte) = []byte(`[{"type":"tag_interest","description":"matches tag"}]`)
						*dest[4].(**time.Time) = &now
						*dest[5].(**time.Time) = &now
						*dest[6].(**time.Time) = nil
						*dest[7].(*time.Time) = now
						*dest[8].(*int) = 2
						*dest[9].(*uuid.UUID) = tenantID
						*dest[10].(*int) = 3
						*dest[11].(**string) = &title
						*dest[12].(**string) = &summary
						*dest[13].(*[]byte) = []byte(`["go"]`)
						*dest[14].(*[]byte) = []byte(`[{"code":"new_unread"}]`)
						*dest[15].(**float64) = &score
						*dest[16].(**time.Time) = &now
						*dest[17].(**string) = &summaryState
						*dest[18].(**string) = &url
						*dest[19].(**string) = &itemType
						*dest[20].(**uuid.UUID) = &refID
						return nil
					},
				},
			}, nil
		},
	}
	repo := &Repository{pool: mock}

	candidates, err := repo.GetRecallCandidates(context.Background(), userID, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.NotNil(t, candidates[0].Item)
	assert.Equal(t, tenantID, candidates[0].Item.TenantID)
	assert.Equal(t, 3, candidates[0].Item.ProjectionVersion)
	assert.Equal(t, title, candidates[0].Item.Title)
}

func TestMapTodayDigestRow_Pure(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		raw       rawTodayDigestRow
		wantTags  []string
		wantRecap bool
		wantPulse bool
	}{
		{
			name: "valid top tags",
			raw: rawTodayDigestRow{
				UserID:                userID,
				DigestDate:            date,
				NewArticles:           12,
				SummarizedArticles:    10,
				UnsummarizedArticles:  2,
				TopTagsJSON:           []byte(`["golang", "architecture"]`),
				UpdatedAt:             updatedAt,
				WeeklyRecapAvailable:  true,
				EveningPulseAvailable: false,
			},
			wantTags:  []string{"golang", "architecture"},
			wantRecap: true,
			wantPulse: false,
		},
		{
			name: "empty tags json",
			raw: rawTodayDigestRow{
				UserID:                userID,
				DigestDate:            date,
				NewArticles:           3,
				SummarizedArticles:    3,
				UnsummarizedArticles:  0,
				TopTagsJSON:           nil,
				UpdatedAt:             updatedAt,
				WeeklyRecapAvailable:  false,
				EveningPulseAvailable: true,
			},
			wantTags:  nil,
			wantRecap: false,
			wantPulse: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			digest := mapTodayDigestRow(tc.raw)
			assert.Equal(t, tc.raw.UserID, digest.UserID)
			assert.Equal(t, tc.raw.DigestDate, digest.DigestDate)
			assert.Equal(t, tc.raw.NewArticles, digest.NewArticles)
			assert.Equal(t, tc.raw.SummarizedArticles, digest.SummarizedArticles)
			assert.Equal(t, tc.raw.UnsummarizedArticles, digest.UnsummarizedArticles)
			assert.Equal(t, tc.wantTags, digest.TopTags)
			assert.Equal(t, tc.wantRecap, digest.WeeklyRecapAvailable)
			assert.Equal(t, tc.wantPulse, digest.EveningPulseAvailable)
			assert.Equal(t, tc.raw.UpdatedAt, digest.UpdatedAt)
		})
	}
}

func TestGetTodayDigest(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)

	t.Run("returns digest when row exists", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						*dest[0].(*uuid.UUID) = userID
						*dest[1].(*time.Time) = date
						*dest[2].(*int) = 8
						*dest[3].(*int) = 6
						*dest[4].(*int) = 2
						*dest[5].(*[]byte) = []byte(`["rust","postgres"]`)
						*dest[6].(*time.Time) = updatedAt
						*dest[7].(*bool) = true
						*dest[8].(*bool) = true
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetTodayDigest(ctx, userID, date)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, userID, got.UserID)
		assert.Equal(t, 8, got.NewArticles)
		assert.Equal(t, 6, got.SummarizedArticles)
		assert.Equal(t, 2, got.UnsummarizedArticles)
		assert.Equal(t, []string{"rust", "postgres"}, got.TopTags)
		assert.True(t, got.WeeklyRecapAvailable)
		assert.True(t, got.EveningPulseAvailable)
	})

	t.Run("returns nil on ErrNoRows", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return pgx.ErrNoRows
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetTodayDigest(ctx, userID, date)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("propagates database error", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return errors.New("db error")
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetTodayDigest(ctx, userID, date)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "GetTodayDigest")
	})
}

func TestMapRecallCandidateRow_Pure(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	tenantID := uuid.New()
	refID := uuid.New()
	suggestAt := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	eligibleAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	pubAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)

	title := "Recall Article Title"
	summary := "Summary of the article"
	score := 0.95
	summaryState := "ready"
	url := "https://example.com/article"
	itemType := "article"

	cases := []struct {
		name        string
		raw         rawRecallCandidateRow
		wantHasItem bool
		wantReasonN int
	}{
		{
			name: "candidate with embedded item and reasons",
			raw: rawRecallCandidateRow{
				UserID:                userID,
				ItemKey:               "article:key-1",
				RecallScore:           0.85,
				ReasonJSON:            []byte(`[{"type":"spaced_repetition","description":"due for review"}]`),
				NextSuggestAt:         &suggestAt,
				FirstEligibleAt:       &eligibleAt,
				SnoozedUntil:          nil,
				UpdatedAt:             updatedAt,
				ProjectionVersion:     1,
				ItemTenantID:          tenantID,
				ItemProjectionVersion: 4,
				ItemTitle:             &title,
				ItemSummary:           &summary,
				ItemTagsJSON:          []byte(`["distributed-systems"]`),
				ItemWhyJSON:           []byte(`[{"code":"deep_dive"}]`),
				ItemScore:             &score,
				ItemPublishedAt:       &pubAt,
				ItemSummaryState:      &summaryState,
				ItemURL:               &url,
				ItemType:              &itemType,
				ItemPrimaryRefID:      &refID,
			},
			wantHasItem: true,
			wantReasonN: 1,
		},
		{
			name: "candidate without embedded home item",
			raw: rawRecallCandidateRow{
				UserID:            userID,
				ItemKey:           "article:key-2",
				RecallScore:       0.5,
				ReasonJSON:        nil,
				NextSuggestAt:     &suggestAt,
				FirstEligibleAt:   &eligibleAt,
				SnoozedUntil:      nil,
				UpdatedAt:         updatedAt,
				ProjectionVersion: 1,
				ItemTitle:         nil,
			},
			wantHasItem: false,
			wantReasonN: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mapRecallCandidateRow(tc.raw)
			assert.Equal(t, tc.raw.UserID, c.UserID)
			assert.Equal(t, tc.raw.ItemKey, c.ItemKey)
			assert.Equal(t, tc.raw.RecallScore, c.RecallScore)
			assert.Len(t, c.Reasons, tc.wantReasonN)

			if tc.wantHasItem {
				require.NotNil(t, c.Item)
				assert.Equal(t, tenantID, c.Item.TenantID)
				assert.Equal(t, 4, c.Item.ProjectionVersion)
				assert.Equal(t, title, c.Item.Title)
				assert.Equal(t, summary, c.Item.SummaryExcerpt)
				assert.Equal(t, score, c.Item.Score)
				assert.Equal(t, &pubAt, c.Item.PublishedAt)
				assert.Equal(t, summaryState, c.Item.SummaryState)
				assert.Equal(t, url, c.Item.URL)
				assert.Equal(t, itemType, c.Item.ItemType)
				assert.Equal(t, &refID, c.Item.PrimaryRefID)
				assert.Equal(t, []string{"distributed-systems"}, c.Item.Tags)
				require.Len(t, c.Item.WhyReasons, 1)
				assert.Equal(t, "deep_dive", c.Item.WhyReasons[0].Code)
			} else {
				assert.Nil(t, c.Item)
			}
		})
	}
}

func TestGetProjectionFreshness_DoesNotFilterByOccurredAt(t *testing.T) {
	ctx := context.Background()
	ts := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)

	t.Run("returns freshness timestamp and checks SQL", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						*dest[0].(*time.Time) = ts
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetProjectionFreshness(ctx, "knowledge-home-projector")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ts, *got)
		require.Len(t, mock.queryRowCalls, 1)

		sql := mock.queryRowCalls[0].SQL
		assert.Contains(t, sql, "e.event_seq > c.last_event_seq",
			"GetProjectionFreshness must check whether any event_seq exceeds last_event_seq")
		assert.NotContains(t, sql, "occurred_at",
			"GetProjectionFreshness must not filter out backfilled/replayed unprojected events with old occurred_at timestamps")
	})

	t.Run("returns nil on ErrNoRows", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return pgx.ErrNoRows
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetProjectionFreshness(ctx, "missing-projector")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("propagates database error", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return errors.New("connection lost")
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		got, err := repo.GetProjectionFreshness(ctx, "knowledge-home-projector")
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "GetProjectionFreshness")
	})
}
