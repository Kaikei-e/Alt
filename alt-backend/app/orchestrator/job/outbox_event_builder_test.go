package job

import (
	"encoding/json"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArticleUpsertPayload(t *testing.T) {
	tests := []struct {
		name        string
		payload     []byte
		wantErr     bool
		errContains string
		wantUserID  string
	}{
		{
			name: "valid article upsert payload",
			payload: []byte(`{
				"article_id": "art-1",
				"url": "https://example.com/art1",
				"title": "Title 1",
				"body": "Body content",
				"user_id": "usr-123"
			}`),
			wantErr:    false,
			wantUserID: "usr-123",
		},
		{
			name: "missing user id",
			payload: []byte(`{
				"article_id": "art-2",
				"url": "https://example.com/art2",
				"title": "Title 2",
				"body": "Body content",
				"user_id": ""
			}`),
			wantErr:     true,
			errContains: "missing owner user_id",
		},
		{
			name: "whitespace user id",
			payload: []byte(`{
				"article_id": "art-3",
				"user_id": "   "
			}`),
			wantErr:     true,
			errContains: "missing owner user_id",
		},
		{
			name:        "invalid json",
			payload:     []byte(`{invalid-json`),
			wantErr:     true,
			errContains: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArticleUpsertPayload(tt.payload)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUserID, got.UserID)
		})
	}
}

func TestBuildArticleCreatedKnowledgeEvent(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	validUserUUID := uuid.New()
	validUserID := validUserUUID.String()
	articleID := uuid.New().String()
	updatedAt := time.Date(2026, 9, 24, 15, 30, 0, 0, time.UTC).Format(time.RFC3339)

	tests := []struct {
		name        string
		payload     []byte
		now         time.Time
		wantSkip    bool
		wantErr     bool
		errContains string
		checkEvent  func(t *testing.T, ev *domain.KnowledgeEvent)
	}{
		{
			name: "valid payload with updated_at",
			payload: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"article_id": articleID,
					"url":        "https://example.com/news",
					"title":      "News Title",
					"user_id":    validUserID,
					"updated_at": updatedAt,
				})
				return b
			}(),
			now:      now,
			wantSkip: false,
			wantErr:  false,
			checkEvent: func(t *testing.T, ev *domain.KnowledgeEvent) {
				assert.Equal(t, domain.EventArticleCreated, ev.EventType)
				assert.Equal(t, articleID, ev.AggregateID)
				assert.Equal(t, validUserUUID, ev.TenantID)
				require.NotNil(t, ev.UserID)
				assert.Equal(t, validUserUUID, *ev.UserID)
				expectedOccurred, _ := time.Parse(time.RFC3339, updatedAt)
				assert.Equal(t, expectedOccurred, ev.OccurredAt)
			},
		},
		{
			name: "valid payload without updated_at falls back to now",
			payload: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"article_id": articleID,
					"url":        "https://example.com/fallback",
					"title":      "Fallback Title",
					"user_id":    validUserID,
					"updated_at": "",
				})
				return b
			}(),
			now:      now,
			wantSkip: false,
			wantErr:  false,
			checkEvent: func(t *testing.T, ev *domain.KnowledgeEvent) {
				assert.Equal(t, now, ev.OccurredAt)
			},
		},
		{
			name: "invalid user_id returns skip sentinel error",
			payload: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"article_id": articleID,
					"url":        "https://example.com/skip",
					"title":      "Skip Title",
					"user_id":    "invalid-not-uuid",
					"updated_at": updatedAt,
				})
				return b
			}(),
			now:      now,
			wantSkip: true,
			wantErr:  true,
		},
		{
			name:        "malformed json returns error",
			payload:     []byte(`{malformed`),
			now:         now,
			wantErr:     true,
			errContains: "unmarshal outbox payload",
		},
		{
			name: "non-RFC3339 updated_at returns error",
			payload: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"article_id": articleID,
					"url":        "https://example.com/err",
					"title":      "Bad Date Title",
					"user_id":    validUserID,
					"updated_at": "not-a-valid-date",
				})
				return b
			}(),
			now:         now,
			wantErr:     true,
			errContains: "parse outbox updated_at for occurred_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := buildArticleCreatedKnowledgeEvent(tt.payload, tt.now)
			if tt.wantSkip {
				assert.ErrorIs(t, err, errSkipInvalidUserID)
				assert.Nil(t, ev)
				return
			}
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ev)
			if tt.checkEvent != nil {
				tt.checkEvent(t, ev)
			}
		})
	}
}
