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

func TestBuildArticleCreatedWrites(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	publishedAtStr := "2026-08-01T10:00:00Z"

	tests := []struct {
		name         string
		evt          sovereign_db.KnowledgeEvent
		version      int
		wantErr      bool
		wantTitle    string
		wantPubAt    bool
		wantUserID   uuid.UUID
		wantSummStat string
	}{
		{
			name: "valid with published_at and user_id",
			evt: sovereign_db.KnowledgeEvent{
				TenantID:   tenantID,
				UserID:     &userID,
				OccurredAt: occurredAt,
				EventSeq:   10,
				Payload: mustJSON(t, map[string]any{
					"article_id":   articleID.String(),
					"title":        "Go SOLID Refactor",
					"published_at": publishedAtStr,
					"url":          "https://example.com/solid",
				}),
			},
			version:      3,
			wantErr:      false,
			wantTitle:    "Go SOLID Refactor",
			wantPubAt:    true,
			wantUserID:   userID,
			wantSummStat: summaryStatePending,
		},
		{
			name: "valid without published_at and nil user_id (tenant fallback)",
			evt: sovereign_db.KnowledgeEvent{
				TenantID:   tenantID,
				UserID:     nil,
				OccurredAt: occurredAt,
				EventSeq:   11,
				Payload: mustJSON(t, map[string]any{
					"article_id": articleID.String(),
					"title":      "Tenant Event",
					"url":        "https://example.com/tenant",
				}),
			},
			version:      3,
			wantErr:      false,
			wantTitle:    "Tenant Event",
			wantPubAt:    false,
			wantUserID:   tenantID,
			wantSummStat: summaryStatePending,
		},
		{
			name: "invalid json payload",
			evt: sovereign_db.KnowledgeEvent{
				TenantID: tenantID,
				Payload:  json.RawMessage(`{not-json}`),
			},
			version: 3,
			wantErr: true,
		},
		{
			name: "invalid article uuid",
			evt: sovereign_db.KnowledgeEvent{
				TenantID: tenantID,
				Payload: mustJSON(t, map[string]any{
					"article_id": "not-a-uuid",
				}),
			},
			version: 3,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, digest, err := buildArticleCreatedWrites(tt.evt, tt.version)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTitle, item.Title)
			assert.Equal(t, tt.wantUserID, item.UserID)
			assert.Equal(t, tt.wantSummStat, item.SummaryState)
			assert.Equal(t, tt.version, item.ProjectionVersion)
			assert.Equal(t, scoreOpMax, item.ScoreOp)
			assert.Equal(t, baseQualityScore, item.Score)
			if tt.wantPubAt {
				assert.NotNil(t, item.PublishedAt)
			} else {
				assert.Nil(t, item.PublishedAt)
			}
			assert.Equal(t, 1, digest.NewArticles)
			assert.Equal(t, 1, digest.UnsummarizedArticles)
			assert.Equal(t, tt.evt.EventSeq, digest.LastEventSeq)
		})
	}
}

func TestIsHTTPURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"http url", "http://example.com/item", true},
		{"https url", "https://example.com/item", true},
		{"http with port", "http://localhost:8080/foo", true},
		{"javascript scheme", "javascript:alert(1)", false},
		{"ftp scheme", "ftp://files.example.com", false},
		{"data scheme", "data:text/plain;base64,SGVsbG8=", false},
		{"empty string", "", false},
		{"whitespace string", "   ", false},
		{"no host", "https://", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isHTTPURL(tt.url))
		})
	}
}

func TestBuildURLPatch(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()

	tests := []struct {
		name       string
		evt        sovereign_db.KnowledgeEvent
		payload    articleUrlBackfilledPayload
		version    int
		wantErr    bool
		wantUserID string
		wantItem   string
		wantURL    string
	}{
		{
			name: "valid patch with user_id",
			evt: sovereign_db.KnowledgeEvent{
				TenantID: tenantID,
				UserID:   &userID,
			},
			payload: articleUrlBackfilledPayload{
				ArticleID: articleID.String(),
				URL:       "https://example.com/corrected",
			},
			version:    2,
			wantErr:    false,
			wantUserID: userID.String(),
			wantItem:   fmt.Sprintf("article:%s", articleID),
			wantURL:    "https://example.com/corrected",
		},
		{
			name: "valid patch with tenant fallback",
			evt: sovereign_db.KnowledgeEvent{
				TenantID: tenantID,
				UserID:   nil,
			},
			payload: articleUrlBackfilledPayload{
				ArticleID: articleID.String(),
				URL:       "https://example.com/tenant-corrected",
			},
			version:    4,
			wantErr:    false,
			wantUserID: tenantID.String(),
			wantItem:   fmt.Sprintf("article:%s", articleID),
			wantURL:    "https://example.com/tenant-corrected",
		},
		{
			name: "invalid article uuid",
			evt: sovereign_db.KnowledgeEvent{
				TenantID: tenantID,
			},
			payload: articleUrlBackfilledPayload{
				ArticleID: "invalid-uuid",
				URL:       "https://example.com/invalid",
			},
			version: 2,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := buildURLPatch(tt.evt, tt.payload, tt.version)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUserID, patch.UserID)
			assert.Equal(t, tt.wantItem, patch.ItemKey)
			assert.Equal(t, tt.version, patch.ProjectionVersion)
			assert.Equal(t, tt.wantURL, patch.URL)
		})
	}
}

func TestBuildSummaryVersionCreatedWrites(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		summaryText    string
		invalidJSON    bool
		invalidUUID    bool
		wantErr        bool
		wantState      string
		wantExcerpt    string
		wantSummCount  int
		wantUnsummDiff int
		wantWhyCode    string
	}{
		{
			name:           "ready summary",
			summaryText:    "Summary available",
			wantErr:        false,
			wantState:      summaryStateReady,
			wantExcerpt:    "Summary available",
			wantSummCount:  1,
			wantUnsummDiff: -1,
			wantWhyCode:    whySummaryCompleted,
		},
		{
			name:           "empty summary stays pending",
			summaryText:    "",
			wantErr:        false,
			wantState:      summaryStatePending,
			wantExcerpt:    "",
			wantSummCount:  0,
			wantUnsummDiff: 0,
			wantWhyCode:    whyNewUnread,
		},
		{
			name:        "invalid json",
			invalidJSON: true,
			wantErr:     true,
		},
		{
			name:        "invalid article uuid",
			invalidUUID: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body json.RawMessage
			if tt.invalidJSON {
				body = json.RawMessage(`{not-json`)
			} else if tt.invalidUUID {
				body = mustJSON(t, map[string]any{"article_id": "not-uuid", "summary_text": "abc"})
			} else {
				body = mustJSON(t, map[string]any{"article_id": articleID.String(), "summary_text": tt.summaryText})
			}

			evt := sovereign_db.KnowledgeEvent{
				TenantID:   tenantID,
				UserID:     &userID,
				OccurredAt: occurredAt,
				EventSeq:   50,
				Payload:    body,
			}
			item, digest, err := buildSummaryVersionCreatedWrites(evt, 1)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, item.SummaryState)
			assert.Equal(t, tt.wantExcerpt, item.SummaryExcerpt)
			assert.Equal(t, 0.8, item.Score)
			assert.Equal(t, scoreOpMax, item.ScoreOp)
			assert.Equal(t, tt.wantSummCount, digest.SummarizedArticles)
			assert.Equal(t, tt.wantUnsummDiff, digest.UnsummarizedArticles)
			assert.Equal(t, int64(50), digest.LastEventSeq)
		})
	}
}

func TestBuildTagSetVersionCreatedWrites(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		tags        []string
		invalidJSON bool
		invalidUUID bool
		wantErr     bool
		wantDigest  bool
	}{
		{
			name:       "non-empty tags",
			tags:       []string{"go", "solid"},
			wantErr:    false,
			wantDigest: true,
		},
		{
			name:       "empty tags",
			tags:       []string{},
			wantErr:    false,
			wantDigest: false,
		},
		{
			name:        "invalid json",
			invalidJSON: true,
			wantErr:     true,
		},
		{
			name:        "invalid uuid",
			invalidUUID: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body json.RawMessage
			if tt.invalidJSON {
				body = json.RawMessage(`{broken`)
			} else if tt.invalidUUID {
				body = mustJSON(t, map[string]any{"article_id": "bad", "tags": []string{"tag"}})
			} else {
				body = mustJSON(t, map[string]any{"article_id": articleID.String(), "tags": tt.tags})
			}

			evt := sovereign_db.KnowledgeEvent{
				TenantID:   tenantID,
				UserID:     &userID,
				OccurredAt: occurredAt,
				EventSeq:   60,
				Payload:    body,
			}
			item, digest, err := buildTagSetVersionCreatedWrites(evt, 1)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.tags, item.Tags)
			assert.Equal(t, 0.7, item.Score)
			assert.Equal(t, scoreOpMax, item.ScoreOp)
			if tt.wantDigest {
				require.NotNil(t, digest)
				assert.Equal(t, tt.tags, digest.TopTags)
				assert.Equal(t, int64(60), digest.LastEventSeq)
			} else {
				assert.Nil(t, digest)
			}
		})
	}
}
