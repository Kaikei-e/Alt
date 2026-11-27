//go:build integration

package sovereign_db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

func mustMarshalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func TestUpsertKnowledgeHomeItem_OnConflictMergeSemanticsAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()

	baseAt := time.Date(2026, time.October, 20, 10, 0, 0, 0, time.UTC)

	// 1. Initial ArticleCreated-style upsert: score 0.5 (score_op: "max"), summary_state: "pending".
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"primary_ref_id":     articleID,
		"title":              "Distributed Consensus in Go",
		"url":                "https://example.com/consensus",
		"why_reasons":        []map[string]string{{"code": "new_unread", "reason": "New article"}},
		"score":              0.5,
		"score_op":           "max",
		"freshness_at":       baseAt,
		"published_at":       baseAt,
		"generated_at":       baseAt,
		"updated_at":         baseAt,
		"projection_version": 1,
		"summary_state":      "pending",
	})))

	items, _, _, err := repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InDelta(t, 0.5, items[0].Score, 1e-6)
	assert.Equal(t, "pending", items[0].SummaryState)
	assert.Equal(t, []WhyReason{{Code: "new_unread"}}, items[0].WhyReasons)

	// 2. SummaryVersionCreated-style upsert: boosts score 0.5 -> 0.8 (score_op: "max"),
	// transitions summary_state -> "ready", preserves title when EXCLUDED.title is empty,
	// and unions why_json deduplicating "new_unread".
	t1 := baseAt.Add(5 * time.Minute)
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"title":              "",
		"summary_excerpt":    "Raft and Paxos compared in production.",
		"why_reasons":        []map[string]string{{"code": "new_unread", "reason": "Duplicate"}, {"code": "summary_completed", "reason": "Summary ready"}},
		"score":              0.8,
		"score_op":           "max",
		"generated_at":       t1,
		"updated_at":         t1,
		"projection_version": 1,
		"summary_state":      "ready",
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InDelta(t, 0.8, items[0].Score, 1e-6, "score_op max must boost 0.5 to 0.8")
	assert.Equal(t, "ready", items[0].SummaryState)
	assert.Equal(t, "Distributed Consensus in Go", items[0].Title, "empty EXCLUDED.title must not overwrite stored title")
	assert.Equal(t, "Raft and Paxos compared in production.", items[0].SummaryExcerpt)
	assert.Equal(t, []WhyReason{{Code: "new_unread"}, {Code: "summary_completed"}}, items[0].WhyReasons,
		"why_json union must deduplicate reasons by code")

	// 3. TagSetVersionCreated-style upsert: score 0.7 with score_op "max" must preserve higher 0.8,
	// summary_state "" or "pending" must not downgrade "ready" (GREATEST), and why_json deduplicates by code.
	t2 := baseAt.Add(10 * time.Minute)
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"tags":               []string{"go", "distributed-systems"},
		"why_reasons":        []map[string]string{{"code": "new_unread", "reason": "Dup again"}, {"code": "in_weekly_recap", "reason": "Recap"}},
		"score":              0.7,
		"score_op":           "max",
		"generated_at":       t2,
		"updated_at":         t2,
		"projection_version": 1,
		"summary_state":      "pending",
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InDelta(t, 0.8, items[0].Score, 1e-6, "score_op max must preserve 0.8 when 0.7 arrives")
	assert.Equal(t, "ready", items[0].SummaryState, "monotonic GREATEST must not downgrade ready to pending")
	assert.Equal(t, []string{"go", "distributed-systems"}, items[0].Tags)
	assert.Equal(t, []WhyReason{
		{Code: "in_weekly_recap"},
		{Code: "new_unread"},
		{Code: "summary_completed"},
	}, items[0].WhyReasons, "why_json must union and deduplicate by code in deterministic order")

	// 4. Supersede/reason-merge upsert with score_op "" and summary_state "":
	// preserves existing score 0.8 and summary_state "ready".
	t3 := baseAt.Add(15 * time.Minute)
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"score":              0.0,
		"score_op":           "",
		"supersede_state":    "summary_updated",
		"superseded_at":      t3,
		"previous_ref_json":  `{"previous_summary_excerpt":"old"}`,
		"generated_at":       t3,
		"updated_at":         t3,
		"projection_version": 1,
		"summary_state":      "",
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InDelta(t, 0.8, items[0].Score, 1e-6, "empty score_op must preserve existing 0.8 score")
	assert.Equal(t, "ready", items[0].SummaryState, "monotonic GREATEST must not downgrade ready to empty string")
	assert.Equal(t, "summary_updated", items[0].SupersedeState)

	// 5. HomeItemOpened-style upsert with score_op "set": lowers 0.8 to 0.1.
	t4 := baseAt.Add(20 * time.Minute)
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"score":              0.1,
		"score_op":           "set",
		"last_interacted_at": t4,
		"generated_at":       t4,
		"updated_at":         t4,
		"projection_version": 1,
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.InDelta(t, 0.1, items[0].Score, 1e-6, "score_op set must lower 0.8 to 0.1 on HomeItemOpened")
	assert.Equal(t, "ready", items[0].SummaryState)

	// 6. Cross-tenant isolation: conflicting upsert with mismatched tenant_id returns ErrHomeItemTenantMismatch.
	err = repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          otherTenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"title":              "Hijacked Title",
		"score":              0.99,
		"score_op":           "set",
		"generated_at":       t4,
		"updated_at":         t4,
		"projection_version": 1,
	}))
	require.ErrorIs(t, err, ErrHomeItemTenantMismatch)

	// 7. Empty-title items are excluded by GetKnowledgeHomeItems.
	untitledID := uuid.New()
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           "article:" + untitledID.String(),
		"item_type":          "article",
		"primary_ref_id":     untitledID,
		"title":              "",
		"score":              0.9,
		"score_op":           "max",
		"generated_at":       t4,
		"updated_at":         t4,
		"projection_version": 1,
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	require.Len(t, items, 1, "item with empty title must be excluded from GetKnowledgeHomeItems")
	assert.Equal(t, itemKey, items[0].ItemKey)

	// 8. DismissKnowledgeHomeItem sets dismissed_at, subsequent UpsertKnowledgeHomeItem preserves it,
	// and GetKnowledgeHomeItems excludes the dismissed item.
	dismissedAt := baseAt.Add(30 * time.Minute)
	require.NoError(t, repo.DismissKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           itemKey,
		"projection_version": 1,
		"dismissed_at":       dismissedAt.Format(time.RFC3339Nano),
	})))

	// Subsequent upsert without dismissed_at must not clear the existing dismissed_at.
	t5 := baseAt.Add(35 * time.Minute)
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"score":              0.8,
		"score_op":           "max",
		"generated_at":       t5,
		"updated_at":         t5,
		"projection_version": 1,
	})))

	items, _, _, err = repo.GetKnowledgeHomeItems(ctx, userID, "", 10, nil)
	require.NoError(t, err)
	assert.Empty(t, items, "dismissed item must stay excluded after a subsequent UpsertKnowledgeHomeItem")

	var storedDismissedAt *time.Time
	require.NoError(t, db.QueryRow(ctx,
		`SELECT dismissed_at FROM knowledge_home_items WHERE user_id = $1 AND item_key = $2 AND projection_version = 1`,
		userID, itemKey,
	).Scan(&storedDismissedAt))
	require.NotNil(t, storedDismissedAt, "dismissed_at must be preserved on conflict")
	assert.True(t, dismissedAt.Equal(storedDismissedAt.UTC()))
}

type supersedeColumns struct {
	State      *string
	At         *time.Time
	PreviousJS *string
	Tags       string
	Summary    *string
	Why        string
	Score      float64
}

func readSupersedeColumns(t *testing.T, db *pgxpool.Pool, userID uuid.UUID, itemKey string) supersedeColumns {
	t.Helper()
	var c supersedeColumns
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT supersede_state, superseded_at, previous_ref_json::text, tags_json::text, summary_excerpt, why_json::text, score::float8
		   FROM knowledge_home_items WHERE user_id = $1 AND item_key = $2 AND projection_version = 1`,
		userID, itemKey,
	).Scan(&c.State, &c.At, &c.PreviousJS, &c.Tags, &c.Summary, &c.Why, &c.Score))
	return c
}

func requireState(t *testing.T, got supersedeColumns, want string) {
	t.Helper()
	require.NotNil(t, got.State, "supersede_state is NULL")
	assert.Equal(t, want, *got.State)
}

func requireSupersededAt(t *testing.T, got supersedeColumns, want time.Time) {
	t.Helper()
	require.NotNil(t, got.At, "superseded_at is NULL")
	assert.True(t, want.Equal(*got.At), "superseded_at = %s, want %s", got.At, want)
}

func requirePreviousRef(t *testing.T, got supersedeColumns, want string) {
	t.Helper()
	require.NotNil(t, got.PreviousJS, "previous_ref_json is NULL")
	assert.JSONEq(t, want, *got.PreviousJS)
}

type supersedeFixture struct {
	repo     *Repository
	db       *pgxpool.Pool
	userID   uuid.UUID
	tenantID uuid.UUID
	itemKey  string
}

func (f supersedeFixture) upsert(t *testing.T, at time.Time, fields map[string]any) {
	t.Helper()
	payload := map[string]any{
		"user_id":            f.userID,
		"tenant_id":          f.tenantID,
		"item_key":           f.itemKey,
		"item_type":          "article",
		"score_op":           "",
		"generated_at":       at,
		"updated_at":         at,
		"projection_version": 1,
	}
	for k, v := range fields {
		payload[k] = v
	}
	require.NoError(t, f.repo.UpsertKnowledgeHomeItem(context.Background(), mustMarshalJSON(t, payload)))
}

func (f supersedeFixture) seed(t *testing.T, at time.Time) {
	t.Helper()
	f.upsert(t, at, map[string]any{
		"title":           "Seeded",
		"summary_excerpt": "first summary",
		"tags":            []string{"go"},
		"why_reasons":     []map[string]string{{"code": "new_unread", "reason": "New"}},
		"score":           0.5,
		"score_op":        "max",
		"summary_state":   "pending",
	})
}

func (f supersedeFixture) supersede(t *testing.T, at time.Time, state, prevRef string) {
	t.Helper()
	f.upsert(t, at, map[string]any{
		"supersede_state":   state,
		"superseded_at":     at,
		"previous_ref_json": prevRef,
	})
}

func newSupersedeFixture(t *testing.T, db *pgxpool.Pool) supersedeFixture {
	t.Helper()
	return supersedeFixture{
		repo:     NewRepository(db),
		db:       db,
		userID:   uuid.New(),
		tenantID: uuid.New(),
		itemKey:  "article:" + uuid.NewString(),
	}
}

var (
	supersedeBaseAt = time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)
	supersedeT1     = supersedeBaseAt.Add(1 * time.Minute)
	supersedeT2     = supersedeBaseAt.Add(2 * time.Minute)
	supersedeT3     = supersedeBaseAt.Add(3 * time.Minute)
	supersedeT4     = supersedeBaseAt.Add(4 * time.Minute)
)

func TestUpsertKnowledgeHomeItem_SupersedeColumnsPersistAsPayloadAgainstPostgres(t *testing.T) {
	t.Parallel()
	db := pgtest.NewDB(t)

	t.Run("update of an existing row", func(t *testing.T) {
		f := newSupersedeFixture(t, db)
		f.seed(t, supersedeBaseAt)
		before := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		assert.Nil(t, before.State)
		assert.Nil(t, before.At)
		assert.Nil(t, before.PreviousJS, "an item that was never superseded stores NULL, not '{}' or ''")

		f.supersede(t, supersedeT1, "summary_updated", `{"previous_summary_excerpt":"first summary"}`)

		got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		requireState(t, got, "summary_updated")
		requireSupersededAt(t, got, supersedeT1)
		requirePreviousRef(t, got, `{"previous_summary_excerpt":"first summary"}`)
	})

	t.Run("insert of a new row", func(t *testing.T) {
		f := newSupersedeFixture(t, db)
		f.supersede(t, supersedeT1, "tags_updated", `{"previous_tags":["old"]}`)

		got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		requireState(t, got, "tags_updated")
		requireSupersededAt(t, got, supersedeT1)
		requirePreviousRef(t, got, `{"previous_tags":["old"]}`)
	})
}

func TestUpsertKnowledgeHomeItem_SupersedeColumnsSurviveEachNonSupersedeUpdateAgainstPostgres(t *testing.T) {
	t.Parallel()
	db := pgtest.NewDB(t)

	cases := []struct {
		name   string
		fields map[string]any
		check  func(t *testing.T, got supersedeColumns)
	}{
		{
			name: "summary update",
			fields: map[string]any{
				"summary_excerpt": "second summary",
				"score":           0.8,
				"score_op":        "max",
				"summary_state":   "ready",
			},
			check: func(t *testing.T, got supersedeColumns) {
				require.NotNil(t, got.Summary)
				assert.Equal(t, "second summary", *got.Summary)
			},
		},
		{
			name:   "tag update",
			fields: map[string]any{"tags": []string{"go", "rust"}, "score": 0.7, "score_op": "max"},
			check: func(t *testing.T, got supersedeColumns) {
				assert.JSONEq(t, `["go","rust"]`, got.Tags)
			},
		},
		{
			name:   "reason update",
			fields: map[string]any{"why_reasons": []map[string]string{{"code": "in_weekly_recap", "reason": "Recap"}}},
			check: func(t *testing.T, got supersedeColumns) {
				assert.JSONEq(t, `[{"code":"in_weekly_recap","reason":"Recap"},{"code":"new_unread","reason":"New"}]`, got.Why)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSupersedeFixture(t, db)
			f.seed(t, supersedeBaseAt)
			f.supersede(t, supersedeT1, "tags_updated", `{"previous_tags":["old"]}`)

			f.upsert(t, supersedeT2, tc.fields)

			got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
			requireState(t, got, "tags_updated")
			requireSupersededAt(t, got, supersedeT1)
			requirePreviousRef(t, got, `{"previous_tags":["old"]}`)
			tc.check(t, got)
		})
	}
}

func TestUpsertKnowledgeHomeItem_SupersedeFoldsMergeAcrossKindsAgainstPostgres(t *testing.T) {
	t.Parallel()
	f := newSupersedeFixture(t, pgtest.NewDB(t))

	f.seed(t, supersedeBaseAt)
	f.upsert(t, supersedeT1, map[string]any{"summary_excerpt": "second summary", "summary_state": "ready", "score": 0.8, "score_op": "max"})

	// Supersede folds send no summary and no tags, so none of them may disturb
	// the content columns while they merge their own.
	f.supersede(t, supersedeT2, "summary_updated", `{"previous_summary_excerpt":"first summary"}`)
	f.supersede(t, supersedeT3, "tags_updated", `{"previous_tags":["go"]}`)
	f.supersede(t, supersedeT4, "reason_updated", `{"previous_why_codes":["new_unread"]}`)

	got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	requireState(t, got, "reason_updated")
	requireSupersededAt(t, got, supersedeT4)
	requirePreviousRef(t, got,
		`{"previous_summary_excerpt":"first summary","previous_tags":["go"],"previous_why_codes":["new_unread"]}`)
	require.NotNil(t, got.Summary)
	assert.Equal(t, "second summary", *got.Summary)
	assert.JSONEq(t, `["go"]`, got.Tags)
	assert.InDelta(t, 0.8, got.Score, 1e-6)

	f.supersede(t, supersedeT4.Add(time.Minute), "summary_updated", `{"previous_summary_excerpt":"second summary"}`)
	got = readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	requirePreviousRef(t, got,
		`{"previous_summary_excerpt":"second summary","previous_tags":["go"],"previous_why_codes":["new_unread"]}`)
}

func TestUpsertKnowledgeHomeItem_SupersedeReplayAndOrderAgainstPostgres(t *testing.T) {
	t.Parallel()
	db := pgtest.NewDB(t)

	summaryEvt := func(t *testing.T, f supersedeFixture) {
		f.supersede(t, supersedeT1, "summary_updated", `{"previous_summary_excerpt":"a"}`)
	}
	tagsEvt := func(t *testing.T, f supersedeFixture) {
		f.supersede(t, supersedeT2, "tags_updated", `{"previous_tags":["b"]}`)
	}

	t.Run("duplicate delivery is idempotent", func(t *testing.T) {
		f := newSupersedeFixture(t, db)
		f.seed(t, supersedeBaseAt)
		summaryEvt(t, f)
		once := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		summaryEvt(t, f)
		twice := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		requireState(t, twice, *once.State)
		requireSupersededAt(t, twice, *once.At)
		requirePreviousRef(t, twice, *once.PreviousJS)
	})

	t.Run("distinct-key previous_ref_json does not depend on order", func(t *testing.T) {
		inOrder := newSupersedeFixture(t, db)
		inOrder.seed(t, supersedeBaseAt)
		summaryEvt(t, inOrder)
		tagsEvt(t, inOrder)

		swapped := newSupersedeFixture(t, db)
		swapped.seed(t, supersedeBaseAt)
		tagsEvt(t, swapped)
		summaryEvt(t, swapped)

		a := readSupersedeColumns(t, inOrder.db, inOrder.userID, inOrder.itemKey)
		b := readSupersedeColumns(t, swapped.db, swapped.userID, swapped.itemKey)
		require.NotNil(t, a.PreviousJS)
		requirePreviousRef(t, b, *a.PreviousJS)
	})

	t.Run("the later-applied supersede wins even when its occurred_at is older", func(t *testing.T) {
		// The driver never looks at event_seq or occurred_at. This is correct
		// only while the single in-process projector folds events in
		// event_seq order; the checkpoint CAS guards the checkpoint value, not
		// the order concurrent writers' upserts land in.
		f := newSupersedeFixture(t, db)
		f.seed(t, supersedeBaseAt)
		tagsEvt(t, f)
		summaryEvt(t, f)

		got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
		requireState(t, got, "summary_updated")
		requireSupersededAt(t, got, supersedeT1)
	})
}

func TestClearSupersedeState_ThenUpsertAgainstPostgres(t *testing.T) {
	t.Parallel()
	f := newSupersedeFixture(t, pgtest.NewDB(t))

	f.seed(t, supersedeBaseAt)
	f.supersede(t, supersedeT1, "summary_updated", `{"previous_summary_excerpt":"first summary"}`)

	require.NoError(t, f.repo.ClearSupersedeState(context.Background(), mustMarshalJSON(t, map[string]any{
		"user_id":            f.userID.String(),
		"tenant_id":          f.tenantID.String(),
		"item_key":           f.itemKey,
		"projection_version": 1,
	})))

	cleared := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	assert.Nil(t, cleared.State)
	assert.Nil(t, cleared.At)
	assert.Nil(t, cleared.PreviousJS)

	f.upsert(t, supersedeT2, map[string]any{"tags": []string{"go", "rust"}})
	stillCleared := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	assert.Nil(t, stillCleared.State, "a later non-supersede upsert must not resurrect cleared supersede data")
	assert.Nil(t, stillCleared.At)
	assert.Nil(t, stillCleared.PreviousJS)

	f.supersede(t, supersedeT3, "tags_updated", `{"previous_tags":["go"]}`)
	again := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	requireState(t, again, "tags_updated")
	requireSupersededAt(t, again, supersedeT3)
	requirePreviousRef(t, again, `{"previous_tags":["go"]}`)
}

// previous_ref_json travels as a text argument; the production pool (default
// cache-statement mode) must round-trip quotes, backslashes and non-ASCII.
func TestUpsertKnowledgeHomeItem_PreviousRefJSONSpecialCharactersRoundTripAgainstPostgres(t *testing.T) {
	t.Parallel()
	f := newSupersedeFixture(t, pgtest.NewDB(t))

	const text = `it's "quoted" \ 日本語`
	prev, err := json.Marshal(map[string]string{"previous_summary_excerpt": text})
	require.NoError(t, err)

	f.seed(t, supersedeBaseAt)
	f.supersede(t, supersedeT1, "summary_updated", string(prev))

	got := readSupersedeColumns(t, f.db, f.userID, f.itemKey)
	require.NotNil(t, got.PreviousJS)
	var stored map[string]string
	require.NoError(t, json.Unmarshal([]byte(*got.PreviousJS), &stored))
	assert.Equal(t, text, stored["previous_summary_excerpt"])
}

func TestUpsertTodayDigest_MonotonicEventSeqAndFloorAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	digestDate := time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)
	dateStr := digestDate.Format(time.DateOnly)
	baseAt := time.Date(2026, time.October, 20, 8, 0, 0, 0, time.UTC)

	// 1. Initial insert with negative unsummarized_articles delta (e.g. summarizing yesterday's article at midnight)
	// must floor unsummarized_articles at 0 via GREATEST(0, $5).
	require.NoError(t, repo.UpsertTodayDigest(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":               userID,
		"digest_date":           dateStr,
		"new_articles":          0,
		"summarized_articles":   1,
		"unsummarized_articles": -1,
		"top_tags":              []string{"rust"},
		"updated_at":            baseAt,
		"last_event_seq":        10,
	})))

	got, err := repo.GetTodayDigest(ctx, userID, digestDate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 0, got.NewArticles)
	assert.Equal(t, 1, got.SummarizedArticles)
	assert.Equal(t, 0, got.UnsummarizedArticles, "initial INSERT must floor unsummarized_articles at 0")
	assert.Equal(t, []string{"rust"}, got.TopTags)

	// 2. Higher last_event_seq accumulates deltas.
	require.NoError(t, repo.UpsertTodayDigest(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":               userID,
		"digest_date":           dateStr,
		"new_articles":          2,
		"summarized_articles":   0,
		"unsummarized_articles": 2,
		"top_tags":              []string{"rust", "postgres"},
		"updated_at":            baseAt.Add(5 * time.Minute),
		"last_event_seq":        20,
	})))

	got, err = repo.GetTodayDigest(ctx, userID, digestDate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.NewArticles)
	assert.Equal(t, 1, got.SummarizedArticles)
	assert.Equal(t, 2, got.UnsummarizedArticles)
	assert.Equal(t, []string{"rust", "postgres"}, got.TopTags)

	// 3. Replaying the same (20) or lower (15) last_event_seq must not double-count.
	for _, replaySeq := range []int64{20, 15} {
		require.NoError(t, repo.UpsertTodayDigest(ctx, mustMarshalJSON(t, map[string]any{
			"user_id":               userID,
			"digest_date":           dateStr,
			"new_articles":          5,
			"summarized_articles":   5,
			"unsummarized_articles": 5,
			"updated_at":            baseAt.Add(10 * time.Minute),
			"last_event_seq":        replaySeq,
		})))
	}

	got, err = repo.GetTodayDigest(ctx, userID, digestDate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.NewArticles, "replaying same or lower last_event_seq must not double-count new_articles")
	assert.Equal(t, 1, got.SummarizedArticles, "replaying same or lower last_event_seq must not double-count summarized_articles")
	assert.Equal(t, 2, got.UnsummarizedArticles, "replaying same or lower last_event_seq must not alter unsummarized_articles")

	// 4. Higher last_event_seq (30) with negative unsummarized delta decrements and floors at 0.
	require.NoError(t, repo.UpsertTodayDigest(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":               userID,
		"digest_date":           dateStr,
		"new_articles":          0,
		"summarized_articles":   1,
		"unsummarized_articles": -1,
		"updated_at":            baseAt.Add(15 * time.Minute),
		"last_event_seq":        30,
	})))

	got, err = repo.GetTodayDigest(ctx, userID, digestDate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.SummarizedArticles)
	assert.Equal(t, 1, got.UnsummarizedArticles)

	require.NoError(t, repo.UpsertTodayDigest(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":               userID,
		"digest_date":           dateStr,
		"new_articles":          0,
		"summarized_articles":   2,
		"unsummarized_articles": -5,
		"updated_at":            baseAt.Add(20 * time.Minute),
		"last_event_seq":        40,
	})))

	got, err = repo.GetTodayDigest(ctx, userID, digestDate)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 4, got.SummarizedArticles)
	assert.Equal(t, 0, got.UnsummarizedArticles, "conflict update must floor unsummarized_articles at 0")
}

func TestGetRecallCandidates_JoinedHomeItemAndSnoozeDismissAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()
	now := time.Now().UTC().Truncate(time.Second)
	pastEligible := now.Add(-30 * time.Minute)

	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"primary_ref_id":     articleID,
		"title":              "Recallable Storage Architecture",
		"summary_excerpt":    "Write-ahead log internals.",
		"tags":               []string{"storage", "wal"},
		"why_reasons":        []map[string]string{{"code": "new_unread", "reason": "New"}},
		"score":              0.8,
		"score_op":           "max",
		"published_at":       pastEligible,
		"generated_at":       pastEligible,
		"updated_at":         pastEligible,
		"projection_version": 1,
		"summary_state":      "ready",
		"url":                "https://example.com/wal",
	})))

	require.NoError(t, repo.UpsertRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID.String(),
		"item_key":           itemKey,
		"recall_score":       0.75,
		"reasons":            []map[string]string{{"type": "opened_before_but_not_revisited", "description": "Opened earlier"}},
		"next_suggest_at":    pastEligible,
		"first_eligible_at":  pastEligible,
		"updated_at":         now,
		"projection_version": 1,
	})))

	candidates, err := repo.GetRecallCandidates(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.NotNil(t, candidates[0].Item, "GetRecallCandidates must populate the joined KnowledgeHomeItem")
	assert.Equal(t, "Recallable Storage Architecture", candidates[0].Item.Title)
	assert.Equal(t, tenantID, candidates[0].Item.TenantID, "joined KnowledgeHomeItem must carry its TenantID")
	assert.Equal(t, 1, candidates[0].Item.ProjectionVersion, "joined KnowledgeHomeItem must carry its ProjectionVersion")

	// Snooze into the future hides the candidate.
	require.NoError(t, repo.SnoozeRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    itemKey,
		"until":       now.Add(2 * time.Hour).Format(time.RFC3339Nano),
		"occurred_at": now.Format(time.RFC3339Nano),
	})))

	candidates, err = repo.GetRecallCandidates(ctx, userID, 10)
	require.NoError(t, err)
	assert.Empty(t, candidates, "candidate snoozed into the future must not be returned")

	// Once snoozed_until is in the past, the candidate is eligible again.
	require.NoError(t, repo.SnoozeRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    itemKey,
		"until":       now.Add(-1 * time.Minute).Format(time.RFC3339Nano),
		"occurred_at": now.Format(time.RFC3339Nano),
	})))

	candidates, err = repo.GetRecallCandidates(ctx, userID, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "candidate whose snoozed_until has elapsed must be returned again")

	// Dismissing the recall candidate excludes it permanently.
	require.NoError(t, repo.DismissRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    itemKey,
		"occurred_at": now.Format(time.RFC3339Nano),
	})))

	candidates, err = repo.GetRecallCandidates(ctx, userID, 10)
	require.NoError(t, err)
	assert.Empty(t, candidates, "dismissed recall candidate must be excluded")
}

func TestRecallCandidate_ProjectionVersionCutoverAgainstPostgres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	itemKey := "article:" + articleID.String()
	baseAt := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	pastEligible := baseAt.Add(-30 * time.Minute)

	// 1. Seed home item and recall candidate at projection_version = 1.
	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"primary_ref_id":     articleID,
		"title":              "Pre-Cutover Recall Item",
		"score":              0.8,
		"score_op":           "max",
		"published_at":       pastEligible,
		"generated_at":       pastEligible,
		"updated_at":         pastEligible,
		"projection_version": 1,
		"summary_state":      "ready",
	})))

	require.NoError(t, repo.UpsertRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID.String(),
		"item_key":           itemKey,
		"recall_score":       0.75,
		"reasons":            []map[string]string{{"type": "opened_before_but_not_revisited", "description": "Opened earlier"}},
		"next_suggest_at":    pastEligible,
		"first_eligible_at":  pastEligible,
		"updated_at":         baseAt,
		"projection_version": 1,
	})))

	// 2. Cut over to projection_version = 2 and drop the v1 home item row so the
	// item exists only at the active version 2 while recall_candidate_view still
	// holds projection_version = 1.
	require.NoError(t, repo.CreateProjectionVersion(ctx, ProjectionVersion{
		Version:     2,
		Description: "v2 cutover",
		Status:      "pending",
		CreatedAt:   baseAt,
	}))
	require.NoError(t, repo.ActivateProjectionVersion(ctx, 2))

	require.NoError(t, repo.UpsertKnowledgeHomeItem(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID,
		"item_key":           itemKey,
		"item_type":          "article",
		"primary_ref_id":     articleID,
		"title":              "Post-Cutover Recall Item",
		"score":              0.85,
		"score_op":           "max",
		"published_at":       pastEligible,
		"generated_at":       pastEligible,
		"updated_at":         baseAt,
		"projection_version": 2,
		"summary_state":      "ready",
	})))
	_, err := db.Exec(ctx, `DELETE FROM knowledge_home_items WHERE user_id = $1 AND item_key = $2 AND projection_version = 1`, userID, itemKey)
	require.NoError(t, err)

	// 3. Snooze, Dismiss, and conflicting Upsert at version 2 must all succeed
	// without returning ErrRecallHomeAssociationDenied.
	require.NoError(t, repo.SnoozeRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    itemKey,
		"until":       baseAt.Add(time.Hour).Format(time.RFC3339Nano),
		"occurred_at": baseAt.Format(time.RFC3339Nano),
	})))

	require.NoError(t, repo.UpsertRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":            userID,
		"tenant_id":          tenantID.String(),
		"item_key":           itemKey,
		"recall_score":       0.9,
		"reasons":            []map[string]string{{"type": "opened_before_but_not_revisited", "description": "Re-evaluated at v2"}},
		"next_suggest_at":    pastEligible,
		"first_eligible_at":  pastEligible,
		"updated_at":         baseAt.Add(5 * time.Minute),
		"projection_version": 2,
	})))

	require.NoError(t, repo.DismissRecallCandidate(ctx, mustMarshalJSON(t, map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    itemKey,
		"occurred_at": baseAt.Add(10 * time.Minute).Format(time.RFC3339Nano),
	})))
}
