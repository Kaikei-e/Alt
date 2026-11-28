package reading_projector

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestProjector_FoldsAnnotationCreateAndDelete(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	annID := uuid.New()
	artID := uuid.New()
	t1 := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)

	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		readingEvt(t, 1, "reading.annotation_created.v1", t1, tenantID, userID, "annotation:"+annID.String(), map[string]any{
			"annotation_id": annID.String(),
			"article_id":    artID.String(),
			"stance":        "contradiction",
			"excerpt":       "Latency dropped by 40%.",
			"note":          "Conflicts with benchmark B from last week.",
			"created_at":    t1.Format(time.RFC3339),
		}),
	})

	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	ann, ok := repo.annotations[annotationKey{userID: userID, annotationID: annID}]
	require.True(t, ok)
	assert.Equal(t, artID, ann.ArticleID)
	assert.Equal(t, "contradiction", ann.Stance)
	assert.Equal(t, "Latency dropped by 40%.", ann.Excerpt)
	assert.Equal(t, "Conflicts with benchmark B from last week.", ann.Note)
	assert.Nil(t, ann.DeletedAt)

	repo.insertEventSorted(readingEvt(t, 2, "reading.annotation_deleted.v1", t2, tenantID, userID, "annotation:"+annID.String(), map[string]any{
		"annotation_id": annID.String(),
		"deleted_at":    t2.Format(time.RFC3339),
	}))
	require.NoError(t, p.RunBatch(context.Background()))

	ann = repo.annotations[annotationKey{userID: userID, annotationID: annID}]
	require.NotNil(t, ann.DeletedAt)
	assert.True(t, t2.Equal(*ann.DeletedAt))
}

func TestProjector_FoldsThemeCurationCreateAndRemove(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	curID := uuid.New()
	themeID := uuid.New()
	t1 := time.Date(2026, 11, 28, 11, 0, 0, 0, time.UTC)
	t2 := t1.Add(3 * time.Minute)

	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		readingEvt(t, 1, "reading.theme_item_curated.v1", t1, tenantID, userID, "theme_curation:"+curID.String(), map[string]any{
			"curation_id":           curID.String(),
			"theme_id":              themeID.String(),
			"action_kind":           "promote_augur_insight",
			"target_hash":           "abc123",
			"target_text":           "How does Raft recover from asymmetric partitions?",
			"user_note":             "Pre-Vote prevents term inflation.",
			"augur_conversation_id": "conv-1",
			"created_at":            t1.Format(time.RFC3339),
		}),
	})

	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	cur, ok := repo.themeCurations[curationKey{userID: userID, curationID: curID}]
	require.True(t, ok)
	assert.Equal(t, themeID, cur.ThemeID)
	assert.Equal(t, "promote_augur_insight", cur.ActionKind)
	assert.Equal(t, "Pre-Vote prevents term inflation.", cur.UserNote)
	assert.Equal(t, "conv-1", cur.AugurConversationID)
	assert.Nil(t, cur.DeletedAt)

	repo.insertEventSorted(readingEvt(t, 2, "reading.theme_curation_removed.v1", t2, tenantID, userID, "theme_curation:"+curID.String(), map[string]any{
		"curation_id": curID.String(),
		"removed_at":  t2.Format(time.RFC3339),
	}))
	require.NoError(t, p.RunBatch(context.Background()))

	cur = repo.themeCurations[curationKey{userID: userID, curationID: curID}]
	require.NotNil(t, cur.DeletedAt)
	assert.True(t, t2.Equal(*cur.DeletedAt))
}
