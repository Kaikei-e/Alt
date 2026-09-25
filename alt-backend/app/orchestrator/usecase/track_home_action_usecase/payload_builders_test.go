package track_home_action_usecase

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUserActionPayload(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		metaJSON   string
	}{
		{
			name:       "empty metadata",
			actionType: "open",
			metaJSON:   "",
		},
		{
			name:       "with json metadata",
			actionType: "tag_click",
			metaJSON:   `{"tag":"golang"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildUserActionPayload(tt.actionType, tt.metaJSON)
			var parsed map[string]string
			err := json.Unmarshal(payload, &parsed)
			require.NoError(t, err)
			assert.Equal(t, tt.actionType, parsed["action_type"])
			assert.Equal(t, tt.metaJSON, parsed["metadata_json"])
		})
	}
}

func TestBuildKnowledgePayload(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		actionType string
		itemKey    string
		url        string
		wantURL    bool
	}{
		{
			name:       "without url",
			actionType: "open",
			itemKey:    "article:123",
			url:        "",
			wantURL:    false,
		},
		{
			name:       "with url",
			actionType: "open",
			itemKey:    "article:123",
			url:        "https://example.com/art",
			wantURL:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildKnowledgePayload(tt.actionType, tt.itemKey, userID, tenantID, now, tt.url)
			var parsed map[string]string
			err := json.Unmarshal(payload, &parsed)
			require.NoError(t, err)
			assert.Equal(t, tt.actionType, parsed["action_type"])
			assert.Equal(t, tt.itemKey, parsed["item_key"])
			assert.Equal(t, userID.String(), parsed["user_id"])
			assert.Equal(t, tenantID.String(), parsed["tenant_id"])
			assert.Equal(t, now.Format(time.RFC3339), parsed["opened_at"])
			if tt.wantURL {
				assert.Equal(t, tt.url, parsed["url"])
			} else {
				_, hasURL := parsed["url"]
				assert.False(t, hasURL)
			}
		})
	}
}

func TestBuildRecallSignalPayload(t *testing.T) {
	tests := []struct {
		name       string
		actionType string
		metaJSON   string
		check      func(t *testing.T, payload map[string]any)
	}{
		{
			name:       "no metadata",
			actionType: "open",
			metaJSON:   "",
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "home_action", payload["source"])
				assert.Equal(t, "open", payload["action_type"])
				assert.Nil(t, payload["query"])
				assert.Nil(t, payload["tag"])
			},
		},
		{
			name:       "with query and tag",
			actionType: "search_click",
			metaJSON:   `{"query":"go 1.26","tag":"release"}`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "go 1.26", payload["search_query"])
				assert.Equal(t, "release", payload["tag"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildRecallSignalPayload(tt.actionType, tt.metaJSON)
			tt.check(t, payload)
		})
	}
}
