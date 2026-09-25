package sovereign_db

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCutoffFromTimeWindowAt_Table(t *testing.T) {
	t.Parallel()
	anchor := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		window     string
		wantCutoff time.Time
		wantErr    bool
	}{
		{
			name:       "empty window",
			window:     "",
			wantCutoff: time.Time{},
			wantErr:    false,
		},
		{
			name:       "7 days",
			window:     "7d",
			wantCutoff: anchor.Add(-7 * 24 * time.Hour),
			wantErr:    false,
		},
		{
			name:       "30 days",
			window:     "30d",
			wantCutoff: anchor.Add(-30 * 24 * time.Hour),
			wantErr:    false,
		},
		{
			name:       "90 days",
			window:     "90d",
			wantCutoff: anchor.Add(-90 * 24 * time.Hour),
			wantErr:    false,
		},
		{
			name:    "unsupported window",
			window:  "14d",
			wantErr: true,
		},
		{
			name:    "invalid string",
			window:  "all",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cutoffFromTimeWindowAt(anchor, tc.window)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported time window")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantCutoff, got)
		})
	}
}

func TestCursorEncodingDecoding_Table(t *testing.T) {
	t.Parallel()
	anchor := time.Date(2026, 7, 20, 15, 30, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	t.Run("round trip with published_at", func(t *testing.T) {
		encoded := encodeCursor(0.875, &publishedAt, "article:item-1", anchor)
		c, err := decodeCursor(encoded)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.InDelta(t, 0.875, c.RankScore, 1e-6)
		require.NotNil(t, c.PublishedAt)
		assert.Equal(t, publishedAt, *c.PublishedAt)
		assert.Equal(t, "article:item-1", c.ItemKey)
		assert.Equal(t, anchor, c.AsOf)
	})

	t.Run("round trip without published_at", func(t *testing.T) {
		encoded := encodeCursor(0.5, nil, "article:item-2", anchor)
		c, err := decodeCursor(encoded)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.InDelta(t, 0.5, c.RankScore, 1e-6)
		assert.Nil(t, c.PublishedAt)
		assert.Equal(t, "article:item-2", c.ItemKey)
		assert.Equal(t, anchor, c.AsOf)
	})

	decodeErrorCases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "not base64",
			raw:     "not-valid-base64!?",
			wantErr: "decode base64",
		},
		{
			name:    "too few parts",
			raw:     base64.URLEncoding.EncodeToString([]byte("0.5|pub|key")),
			wantErr: "invalid cursor format",
		},
		{
			name:    "invalid rank_score",
			raw:     base64.URLEncoding.EncodeToString([]byte("not-a-number||item-key|2026-07-20T15:30:00Z")),
			wantErr: "parse rank_score",
		},
		{
			name:    "invalid published_at format",
			raw:     base64.URLEncoding.EncodeToString([]byte("0.5|bad-time|item-key|2026-07-20T15:30:00Z")),
			wantErr: "parse published_at",
		},
		{
			name:    "invalid rank_as_of format",
			raw:     base64.URLEncoding.EncodeToString([]byte("0.5||item-key|bad-as-of")),
			wantErr: "parse rank_as_of",
		},
	}

	for _, tc := range decodeErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeCursor(tc.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestBuildKnowledgeHomeFilterClauses_Table(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		filter       *LensFilter
		startArgPos  int
		wantContains []string
		wantExactSQL string
		wantArgLen   int
		wantNextPos  int
	}{
		{
			name:         "nil filter",
			filter:       nil,
			startArgPos:  2,
			wantExactSQL: "",
			wantArgLen:   0,
			wantNextPos:  2,
		},
		{
			name: "query text only",
			filter: &LensFilter{
				QueryText: "distributed",
			},
			startArgPos: 2,
			wantContains: []string{
				"AND khi.item_type = 'article'",
				"khi.title ILIKE $2",
				"COALESCE(khi.summary_excerpt, '') ILIKE $2",
				"tag_name ILIKE $2",
			},
			wantArgLen:  1,
			wantNextPos: 3,
		},
		{
			name: "tags only",
			filter: &LensFilter{
				TagNames: []string{"database", "storage"},
			},
			startArgPos: 3,
			wantContains: []string{
				"AND khi.item_type = 'article'",
				"tag_name = ANY($3)",
			},
			wantArgLen:  1,
			wantNextPos: 4,
		},
		{
			name: "time window only",
			filter: &LensFilter{
				TimeWindow: "7d",
			},
			startArgPos: 2,
			wantContains: []string{
				"AND khi.item_type = 'article'",
				"khi.published_at >= $2",
			},
			wantExactSQL: " AND khi.item_type = 'article' AND khi.published_at >= $2",
			wantArgLen:   1,
			wantNextPos:  3,
		},
		{
			name: "all filter criteria active",
			filter: &LensFilter{
				QueryText:  "consensus",
				TagNames:   []string{"raft"},
				TimeWindow: "30d",
			},
			startArgPos: 3,
			wantContains: []string{
				"AND khi.item_type = 'article'",
				"khi.title ILIKE $3",
				"tag_name = ANY($4)",
				"khi.published_at >= $5",
			},
			wantArgLen:  3,
			wantNextPos: 6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clauseSQL, args, nextPos := buildKnowledgeHomeFilterClauses(tc.filter, cutoff, tc.startArgPos)
			assert.Equal(t, tc.wantArgLen, len(args))
			assert.Equal(t, tc.wantNextPos, nextPos)
			if tc.wantExactSQL != "" || tc.name == "nil filter" {
				assert.Equal(t, tc.wantExactSQL, clauseSQL)
			}
			for _, fragment := range tc.wantContains {
				assert.Contains(t, clauseSQL, fragment)
			}
		})
	}
}

func TestBuildKnowledgeHomeQuery_Pure(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	anchor := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)

	t.Run("first page without filter anchors on now()", func(t *testing.T) {
		sql, args := buildKnowledgeHomeQuery(userID, nil, nil, time.Time{}, 11)
		assert.Contains(t, sql, "now() AS rank_as_of")
		assert.Contains(t, sql, "WHERE khi.user_id = $1")
		assert.Contains(t, sql, "ORDER BY rank_score DESC, COALESCE(khi.published_at, '-infinity') DESC, khi.item_key DESC LIMIT $2")
		require.Len(t, args, 2)
		assert.Equal(t, userID, args[0])
		assert.Equal(t, 11, args[1])
	})

	t.Run("continuation page binds cursor anchor and keyset predicate", func(t *testing.T) {
		cursor := &homeItemCursor{
			RankScore:   0.75,
			PublishedAt: &cutoff,
			ItemKey:     "article:123",
			AsOf:        anchor,
		}
		sql, args := buildKnowledgeHomeQuery(userID, cursor, nil, time.Time{}, 11)
		assert.Contains(t, sql, "$2::timestamptz AS rank_as_of")
		assert.Contains(t, sql, "($3, COALESCE($4::timestamptz, '-infinity'), $5)")
		assert.Contains(t, sql, "LIMIT $6")
		require.Len(t, args, 6)
		assert.Equal(t, userID, args[0])
		assert.Equal(t, anchor, args[1])
		assert.Equal(t, 0.75, args[2])
		assert.Equal(t, &cutoff, args[3])
		assert.Equal(t, "article:123", args[4])
		assert.Equal(t, 11, args[5])
	})

	t.Run("cursor and all filter criteria active asserts full keyset and limit", func(t *testing.T) {
		cursor := &homeItemCursor{
			RankScore:   0.82,
			PublishedAt: &cutoff,
			ItemKey:     "article:456",
			AsOf:        anchor,
		}
		filter := &LensFilter{
			QueryText:  "distributed",
			TagNames:   []string{"consensus", "raft"},
			TimeWindow: "7d",
		}
		sql, args := buildKnowledgeHomeQuery(userID, cursor, filter, cutoff, 10)
		assert.Contains(t, sql, "LIMIT $9")
		assert.Contains(t, sql, "ILIKE $3")
		assert.Contains(t, sql, "ANY($4)")
		assert.Contains(t, sql, ">= $5")
		assert.Contains(t, sql, "($6, COALESCE($7::timestamptz, '-infinity'), $8)")
		require.Len(t, args, 9)
		assert.Equal(t, userID, args[0])
		assert.Equal(t, anchor, args[1])
		assert.Equal(t, "%distributed%", args[2])
		assert.Equal(t, []string{"consensus", "raft"}, args[3])
		assert.Equal(t, cutoff, args[4])
		assert.Equal(t, 0.82, args[5])
		assert.Equal(t, &cutoff, args[6])
		assert.Equal(t, "article:456", args[7])
		assert.Equal(t, 10, args[8])
	})
}

func TestMapKnowledgeHomeItemRow_Pure(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	tenantID := uuid.New()
	item := KnowledgeHomeItem{
		UserID:   userID,
		TenantID: tenantID,
		ItemKey:  "article:test",
		Title:    "Test Title",
	}

	supersede := "superseded"
	prevRef := `{"prior":"item-old"}`
	tagsJSON := []byte(`["go", "clean-architecture"]`)
	whyJSON := []byte(`[{"code":"top_pick","tag":"go"}]`)

	mapped := mapKnowledgeHomeItemRow(item, tagsJSON, whyJSON, &supersede, &prevRef)

	assert.Equal(t, userID, mapped.UserID)
	assert.Equal(t, "Test Title", mapped.Title)
	assert.Equal(t, []string{"go", "clean-architecture"}, mapped.Tags)
	require.Len(t, mapped.WhyReasons, 1)
	assert.Equal(t, "top_pick", mapped.WhyReasons[0].Code)
	assert.Equal(t, "superseded", mapped.SupersedeState)
	assert.Equal(t, prevRef, mapped.PreviousRefJSON)
}

func TestPaginateHomeItems_Pure(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	pub := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	items := []KnowledgeHomeItem{
		{ItemKey: "item-1", PublishedAt: &pub},
		{ItemKey: "item-2", PublishedAt: &pub},
		{ItemKey: "item-3", PublishedAt: &pub},
	}
	scores := []float64{0.9, 0.8, 0.7}

	t.Run("has more when len exceeds limit", func(t *testing.T) {
		trimmed, nextCursor, hasMore := paginateHomeItems(items, scores, asOf, 2)
		assert.True(t, hasMore)
		require.Len(t, trimmed, 2)
		assert.Equal(t, "item-2", trimmed[1].ItemKey)
		assert.NotEmpty(t, nextCursor)

		c, err := decodeCursor(nextCursor)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.InDelta(t, 0.8, c.RankScore, 1e-6)
		assert.Equal(t, "item-2", c.ItemKey)
	})

	t.Run("no more when len within limit", func(t *testing.T) {
		trimmed, nextCursor, hasMore := paginateHomeItems(items[:2], scores[:2], asOf, 2)
		assert.False(t, hasMore)
		assert.Len(t, trimmed, 2)
		assert.Empty(t, nextCursor)
	})
}
