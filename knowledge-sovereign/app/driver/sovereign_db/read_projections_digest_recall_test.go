package sovereign_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestMapRecallCandidateRow_Pure(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
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
				UserID:            userID,
				ItemKey:           "article:key-1",
				RecallScore:       0.85,
				ReasonJSON:        []byte(`[{"type":"spaced_repetition","description":"due for review"}]`),
				NextSuggestAt:     &suggestAt,
				FirstEligibleAt:   &eligibleAt,
				SnoozedUntil:      nil,
				UpdatedAt:         updatedAt,
				ProjectionVersion: 1,
				ItemTitle:         &title,
				ItemSummary:       &summary,
				ItemTagsJSON:      []byte(`["distributed-systems"]`),
				ItemWhyJSON:       []byte(`[{"code":"deep_dive"}]`),
				ItemScore:         &score,
				ItemPublishedAt:   &pubAt,
				ItemSummaryState:  &summaryState,
				ItemURL:           &url,
				ItemType:          &itemType,
				ItemPrimaryRefID:  &refID,
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
