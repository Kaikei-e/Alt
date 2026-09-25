package knowledge_home_projector

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestBuildHomeItemOpenedWrites(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC)
	itemKey := "article:" + uuid.New().String()

	t.Run("valid open", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload:    mustJSON(t, map[string]any{"item_key": itemKey}),
		}
		item, clear, candidate, err := buildHomeItemOpenedWrites(evt, 2)
		require.NoError(t, err)
		assert.Equal(t, 0.1, item.Score)
		assert.Equal(t, scoreOpSet, item.ScoreOp)
		assert.Equal(t, itemKey, item.ItemKey)
		assert.Equal(t, 2, item.ProjectionVersion)
		require.NotNil(t, item.LastInteractedAt)
		assert.True(t, occurredAt.Equal(*item.LastInteractedAt))

		assert.Equal(t, itemKey, clear.ItemKey)
		assert.Equal(t, userID.String(), clear.UserID)
		assert.Equal(t, 2, clear.ProjectionVersion)

		assert.Equal(t, itemKey, candidate.ItemKey)
		assert.Equal(t, userID, candidate.UserID)
		assert.Equal(t, 0.5, candidate.RecallScore)
		require.NotNil(t, candidate.FirstEligibleAt)
		assert.True(t, occurredAt.Add(1*time.Hour).Equal(*candidate.FirstEligibleAt))
	})

	t.Run("invalid json", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{TenantID: tenantID, Payload: json.RawMessage(`{not-json`)}
		_, _, _, err := buildHomeItemOpenedWrites(evt, 2)
		require.Error(t, err)
	})
}

func TestBuildDismissWrite(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 17, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		payload     any
		aggID       string
		invalidJSON bool
		wantErr     bool
		wantKey     string
	}{
		{
			name:    "payload item_key present",
			payload: map[string]any{"item_key": "article:abc"},
			aggID:   "fallback-id",
			wantErr: false,
			wantKey: "article:abc",
		},
		{
			name:    "fallback to aggregate_id when payload item_key empty",
			payload: map[string]any{"item_key": ""},
			aggID:   "article:fallback",
			wantErr: false,
			wantKey: "article:fallback",
		},
		{
			name:    "missing both payload item_key and aggregate_id",
			payload: map[string]any{"item_key": ""},
			aggID:   "",
			wantErr: true,
		},
		{
			name:        "invalid json",
			invalidJSON: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body json.RawMessage
			if tt.invalidJSON {
				body = json.RawMessage(`{broken`)
			} else {
				body = mustJSON(t, tt.payload)
			}
			evt := sovereign_db.KnowledgeEvent{
				TenantID:    tenantID,
				UserID:      &userID,
				AggregateID: tt.aggID,
				OccurredAt:  occurredAt,
				Payload:     body,
			}
			dismiss, err := buildDismissWrite(evt, 5)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, dismiss.ItemKey)
			assert.Equal(t, 5, dismiss.ProjectionVersion)
			assert.Equal(t, userID.String(), dismiss.UserID)
		})
	}
}

func TestBuildSupersededWrites(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)

	t.Run("SummarySuperseded", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload: mustJSON(t, map[string]any{
				"article_id":               articleID.String(),
				"previous_summary_excerpt": "prior text",
			}),
		}
		item, err := buildSummarySupersededWrite(evt, 1)
		require.NoError(t, err)
		assert.Equal(t, supersedeSummaryUpdated, item.SupersedeState)
		assert.Equal(t, []string{}, item.Tags)
		assert.Contains(t, item.PreviousRefJSON, "prior text")
	})

	t.Run("TagSetSuperseded", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload: mustJSON(t, map[string]any{
				"article_id":    articleID.String(),
				"previous_tags": []string{"tag1", "tag2"},
			}),
		}
		item, err := buildTagSetSupersededWrite(evt, 1)
		require.NoError(t, err)
		assert.Equal(t, supersedeTagsUpdated, item.SupersedeState)
		assert.Equal(t, []string{}, item.Tags)
		assert.Contains(t, item.PreviousRefJSON, "tag1")
	})

	t.Run("ReasonMerged with codes and fallback item_key", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload: mustJSON(t, map[string]any{
				"article_id":         articleID.String(),
				"item_key":           "",
				"added_codes":        []string{"pulse_need_to_know"},
				"previous_why_codes": []string{"new_unread"},
			}),
		}
		item, err := buildReasonMergedWrite(evt, 1)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("article:%s", articleID), item.ItemKey)
		assert.Equal(t, supersedeReasonUpdated, item.SupersedeState)
		require.Len(t, item.WhyReasons, 1)
		assert.Equal(t, "pulse_need_to_know", item.WhyReasons[0].Code)
	})
}
