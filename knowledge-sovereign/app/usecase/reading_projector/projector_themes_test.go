package reading_projector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestReadingProjector_ThemeVersionCreatedAndOpened(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	themeID := uuid.New()
	art1 := uuid.New()
	art2 := uuid.New()
	art3 := uuid.New()

	t1 := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	tOpen1 := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	tOpenEarly := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	tOpen2 := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		readingEvt(t, 1, "reading.theme_version_created.v1", t1, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":           themeID.String(),
			"version":            1,
			"title":              "Consensus Systems",
			"member_article_ids": []string{art1.String(), art2.String()},
			"article_count":      2,
			"created_at":         t1.Format(time.RFC3339),
		}),
		readingEvt(t, 2, "reading.theme_opened.v1", tOpen1, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":  themeID.String(),
			"version":   1,
			"opened_at": tOpen1.Format(time.RFC3339),
		}),
		readingEvt(t, 3, "reading.theme_version_created.v1", t2, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":           themeID.String(),
			"version":            2,
			"title":              "Consensus and Replication",
			"member_article_ids": []string{art2.String(), art3.String()},
			"article_count":      2,
			"created_at":         t2.Format(time.RFC3339),
		}),
		// Earlier opened_at arriving late must not shrink last_opened_at/version.
		readingEvt(t, 4, "reading.theme_opened.v1", tOpenEarly, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":  themeID.String(),
			"version":   1,
			"opened_at": tOpenEarly.Format(time.RFC3339),
		}),
		readingEvt(t, 5, "reading.theme_opened.v1", tOpen2, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":  themeID.String(),
			"version":   2,
			"opened_at": tOpen2.Format(time.RFC3339),
		}),
	}

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Equal(t, int64(5), repo.checkpoint.LastEventSeq)

	th, ok := repo.themes[themeKey{userID: userID, themeID: themeID}]
	require.True(t, ok)
	assert.Equal(t, "Consensus and Replication", th.Title)
	assert.Equal(t, 2, th.LatestVersion)
	assert.Equal(t, 2, th.ArticleCount)
	assert.Equal(t, t1, th.CreatedAt)
	assert.Equal(t, t2, th.UpdatedAt)
	require.NotNil(t, th.LastOpenedAt)
	assert.Equal(t, tOpen2, *th.LastOpenedAt)
	assert.Equal(t, 2, th.LastOpenedVersion)

	members := repo.themeMembers[themeKey{userID: userID, themeID: themeID}]
	assert.Equal(t, map[uuid.UUID]int{
		art2: 1,
		art3: 2,
	}, members)
}

func TestReadingProjector_ThemeOpenedBeforeThemeExistsIsNoopAdvancingCheckpoint(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	unknownThemeID := uuid.New()
	t0 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		readingEvt(t, 1, "reading.theme_opened.v1", t0, tenantID, userID, "theme:"+unknownThemeID.String(), map[string]any{
			"theme_id":  unknownThemeID.String(),
			"version":   1,
			"opened_at": t0.Format(time.RFC3339),
		}),
	}

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Equal(t, int64(1), repo.checkpoint.LastEventSeq)
	_, ok := repo.themes[themeKey{userID: userID, themeID: unknownThemeID}]
	assert.False(t, ok)
}

func TestReadingProjector_ThemeReplayIdempotencyOutOfOrderAndOpenedPreservation(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	themeID := uuid.New()
	art1 := uuid.New()
	art2 := uuid.New()
	art3 := uuid.New()

	t1 := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	tOpen := time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)

	v1 := func(seq int64) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, "reading.theme_version_created.v1", t1, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":           themeID.String(),
			"version":            1,
			"title":              "V1",
			"member_article_ids": []string{art1.String(), art2.String()},
			"article_count":      2,
			"created_at":         t1.Format(time.RFC3339),
		})
	}
	v2 := func(seq int64) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, "reading.theme_version_created.v1", t2, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":           themeID.String(),
			"version":            2,
			"title":              "V2",
			"member_article_ids": []string{art2.String(), art3.String()},
			"article_count":      2,
			"created_at":         t2.Format(time.RFC3339),
		})
	}
	opened := func(seq int64) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, "reading.theme_opened.v1", tOpen, tenantID, userID, "theme:"+themeID.String(), map[string]any{
			"theme_id":  themeID.String(),
			"version":   1,
			"opened_at": tOpen.Format(time.RFC3339),
		})
	}

	t.Run("applying same event twice is idempotent", func(t *testing.T) {
		events := []sovereign_db.KnowledgeEvent{v1(1), v1(2), opened(3), opened(4)}
		repo := newFakeReadingRepo(events)
		p := NewProjector(repo, nil, Config{})
		require.NoError(t, p.RunBatch(context.Background()))

		th := repo.themes[themeKey{userID: userID, themeID: themeID}]
		assert.Equal(t, 1, th.LatestVersion)
		assert.Equal(t, t1, th.CreatedAt)
		require.NotNil(t, th.LastOpenedAt)
		assert.Equal(t, tOpen, *th.LastOpenedAt)
		assert.Equal(t, map[uuid.UUID]int{art1: 1, art2: 1}, repo.themeMembers[themeKey{userID: userID, themeID: themeID}])
	})

	t.Run("out-of-order older version after newer version changes nothing", func(t *testing.T) {
		events := []sovereign_db.KnowledgeEvent{v2(1), v1(2)}
		repo := newFakeReadingRepo(events)
		p := NewProjector(repo, nil, Config{})
		require.NoError(t, p.RunBatch(context.Background()))

		th := repo.themes[themeKey{userID: userID, themeID: themeID}]
		assert.Equal(t, "V2", th.Title)
		assert.Equal(t, 2, th.LatestVersion)
		assert.Equal(t, map[uuid.UUID]int{art2: 2, art3: 2}, repo.themeMembers[themeKey{userID: userID, themeID: themeID}])
	})

	t.Run("opened then new version keeps last_opened fields", func(t *testing.T) {
		events := []sovereign_db.KnowledgeEvent{v1(1), opened(2), v2(3)}
		repo := newFakeReadingRepo(events)
		p := NewProjector(repo, nil, Config{})
		require.NoError(t, p.RunBatch(context.Background()))

		th := repo.themes[themeKey{userID: userID, themeID: themeID}]
		assert.Equal(t, 2, th.LatestVersion)
		assert.Equal(t, t1, th.CreatedAt)
		assert.Equal(t, t2, th.UpdatedAt)
		require.NotNil(t, th.LastOpenedAt)
		assert.Equal(t, tOpen, *th.LastOpenedAt)
		assert.Equal(t, 1, th.LastOpenedVersion)
	})

	t.Run("reproject from empty read model reproduces identical state", func(t *testing.T) {
		events := []sovereign_db.KnowledgeEvent{v1(1), opened(2), v2(3)}
		repo1 := newFakeReadingRepo(events)
		require.NoError(t, NewProjector(repo1, nil, Config{}).RunBatch(context.Background()))

		// Simulate rebuild: clear read model and reset checkpoint to 0.
		repo2 := newFakeReadingRepo(events)
		require.NoError(t, NewProjector(repo2, nil, Config{}).RunBatch(context.Background()))

		assert.Equal(t, repo1.themes, repo2.themes)
		assert.Equal(t, repo1.themeMembers, repo2.themeMembers)
	})
}

func TestReadingProjector_ThemeMalformedPayloadsFollowExistingPolicy(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	themeID := uuid.New()
	t0 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	badPayloads := []struct {
		name      string
		eventType string
		payload   json.RawMessage
	}{
		{
			name:      "theme_version_created invalid json",
			eventType: "reading.theme_version_created.v1",
			payload:   json.RawMessage(`{bad`),
		},
		{
			name:      "theme_version_created invalid theme_id",
			eventType: "reading.theme_version_created.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": "not-uuid", "version": 1, "title": "T", "member_article_ids": []string{}, "article_count": 0, "created_at": t0.Format(time.RFC3339)}),
		},
		{
			name:      "theme_version_created version below 1",
			eventType: "reading.theme_version_created.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 0, "title": "T", "member_article_ids": []string{}, "article_count": 0, "created_at": t0.Format(time.RFC3339)}),
		},
		{
			name:      "theme_version_created invalid member_article_id",
			eventType: "reading.theme_version_created.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 1, "title": "T", "member_article_ids": []string{"bad"}, "article_count": 1, "created_at": t0.Format(time.RFC3339)}),
		},
		{
			name:      "theme_version_created invalid created_at",
			eventType: "reading.theme_version_created.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 1, "title": "T", "member_article_ids": []string{}, "article_count": 0, "created_at": "not-time"}),
		},
		{
			name:      "theme_opened invalid json",
			eventType: "reading.theme_opened.v1",
			payload:   json.RawMessage(`{bad`),
		},
		{
			name:      "theme_opened invalid theme_id",
			eventType: "reading.theme_opened.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": "bad", "version": 1, "opened_at": t0.Format(time.RFC3339)}),
		},
		{
			name:      "theme_opened version below 1",
			eventType: "reading.theme_opened.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 0, "opened_at": t0.Format(time.RFC3339)}),
		},
		{
			name:      "theme_opened invalid opened_at",
			eventType: "reading.theme_opened.v1",
			payload:   mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 1, "opened_at": "bad"}),
		},
	}

	for _, tc := range badPayloads {
		t.Run(tc.name, func(t *testing.T) {
			events := []sovereign_db.KnowledgeEvent{
				{
					EventID:     uuid.New(),
					EventSeq:    1,
					OccurredAt:  t0,
					TenantID:    tenantID,
					UserID:      &userID,
					EventType:   tc.eventType,
					AggregateID: "theme:bad",
					Payload:     tc.payload,
				},
			}
			repo := newFakeReadingRepo(events)
			err := NewProjector(repo, nil, Config{}).RunBatch(context.Background())
			require.Error(t, err)
			assert.Equal(t, int64(0), repo.checkpoint.LastEventSeq)
		})
	}

	t.Run("missing user_id skips and advances checkpoint", func(t *testing.T) {
		events := []sovereign_db.KnowledgeEvent{
			{
				EventID:     uuid.New(),
				EventSeq:    1,
				OccurredAt:  t0,
				TenantID:    tenantID,
				UserID:      nil,
				EventType:   "reading.theme_version_created.v1",
				AggregateID: "theme:" + themeID.String(),
				Payload:     mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 1, "title": "T", "member_article_ids": []string{}, "article_count": 0, "created_at": t0.Format(time.RFC3339)}),
			},
			{
				EventID:     uuid.New(),
				EventSeq:    2,
				OccurredAt:  t0,
				TenantID:    tenantID,
				UserID:      nil,
				EventType:   "reading.theme_opened.v1",
				AggregateID: "theme:" + themeID.String(),
				Payload:     mustJSON(t, map[string]any{"theme_id": themeID.String(), "version": 1, "opened_at": t0.Format(time.RFC3339)}),
			},
		}
		repo := newFakeReadingRepo(events)
		require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))
		assert.Equal(t, int64(2), repo.checkpoint.LastEventSeq)
		assert.Empty(t, repo.themes)
	})
}
