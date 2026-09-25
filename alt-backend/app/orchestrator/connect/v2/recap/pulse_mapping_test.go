package recap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	recapv2 "alt/gen/proto/alt/recap/v2"
)

func TestTopicRoleToProto_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.TopicRole
		expected recapv2.TopicRole
	}{
		{"need to know", domain.TopicRoleNeedToKnow, recapv2.TopicRole_TOPIC_ROLE_NEED_TO_KNOW},
		{"trend", domain.TopicRoleTrend, recapv2.TopicRole_TOPIC_ROLE_TREND},
		{"serendipity", domain.TopicRoleSerendipity, recapv2.TopicRole_TOPIC_ROLE_SERENDIPITY},
		{"unknown", domain.TopicRole("unknown"), recapv2.TopicRole_TOPIC_ROLE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, topicRoleToProto(tt.input))
		})
	}
}

func TestPulseStatusToProto_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.PulseStatus
		expected recapv2.PulseStatus
	}{
		{"normal", domain.PulseStatusNormal, recapv2.PulseStatus_PULSE_STATUS_NORMAL},
		{"partial", domain.PulseStatusPartial, recapv2.PulseStatus_PULSE_STATUS_PARTIAL},
		{"quiet day", domain.PulseStatusQuietDay, recapv2.PulseStatus_PULSE_STATUS_QUIET_DAY},
		{"error", domain.PulseStatusError, recapv2.PulseStatus_PULSE_STATUS_ERROR},
		{"unknown", domain.PulseStatus("unknown"), recapv2.PulseStatus_PULSE_STATUS_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, pulseStatusToProto(tt.input))
		})
	}
}

func TestConfidenceToProto_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.Confidence
		expected recapv2.Confidence
	}{
		{"high", domain.ConfidenceHigh, recapv2.Confidence_CONFIDENCE_HIGH},
		{"medium", domain.ConfidenceMedium, recapv2.Confidence_CONFIDENCE_MEDIUM},
		{"low", domain.ConfidenceLow, recapv2.Confidence_CONFIDENCE_LOW},
		{"unknown", domain.Confidence("unknown"), recapv2.Confidence_CONFIDENCE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, confidenceToProto(tt.input))
		})
	}
}

func TestEveningPulseDomainToProto_TableDriven(t *testing.T) {
	now := time.Date(2026, 2, 1, 18, 0, 0, 0, time.UTC)
	tier1 := 3
	trendMult := 2.5
	genre := "Business"

	tests := []struct {
		name           string
		input          *domain.EveningPulse
		expectedTopics int
		hasQuietDay    bool
	}{
		{
			name: "pulse with topics",
			input: &domain.EveningPulse{
				JobID:       "pulse-1",
				Date:        "2026-02-01",
				GeneratedAt: now,
				Status:      domain.PulseStatusNormal,
				Topics: []domain.PulseTopic{
					{
						ClusterID:       1001,
						Role:            domain.TopicRoleNeedToKnow,
						Title:           "Topic 1",
						Rationale:       domain.PulseRationale{Text: "Significant news", Confidence: domain.ConfidenceHigh},
						ArticleCount:    20,
						SourceCount:     5,
						TimeAgo:         "1 hour ago",
						ArticleIDs:      []string{"a1"},
						TopEntities:     []string{"Entity1"},
						SourceNames:     []string{"Source1"},
						Tier1Count:      &tier1,
						TrendMultiplier: &trendMult,
						Genre:           &genre,
						RepresentativeArticles: []domain.RepresentativeArticle{
							{
								ArticleID:   "a1",
								Title:       "Article 1",
								SourceURL:   "https://example.com/a1",
								SourceName:  "Source 1",
								PublishedAt: "2026-02-01T12:00:00Z",
							},
						},
					},
				},
			},
			expectedTopics: 1,
			hasQuietDay:    false,
		},
		{
			name: "quiet day pulse",
			input: &domain.EveningPulse{
				JobID:       "pulse-qd",
				Date:        "2026-02-01",
				GeneratedAt: now,
				Status:      domain.PulseStatusQuietDay,
				QuietDay: &domain.QuietDayInfo{
					Message: "A quiet day in tech",
					WeeklyHighlights: []domain.WeeklyHighlight{
						{
							ID:    "h-1",
							Title: "Highlight 1",
							Date:  "2026-01-30",
							Role:  "need_to_know",
						},
					},
				},
			},
			expectedTopics: 0,
			hasQuietDay:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := eveningPulseDomainToProto(tt.input)
			require.NotNil(t, resp)
			assert.Equal(t, tt.input.JobID, resp.JobId)
			assert.Len(t, resp.Topics, tt.expectedTopics)
			if tt.hasQuietDay {
				assert.NotNil(t, resp.QuietDay)
				assert.Equal(t, tt.input.QuietDay.Message, resp.QuietDay.Message)
				assert.Len(t, resp.QuietDay.WeeklyHighlights, len(tt.input.QuietDay.WeeklyHighlights))
			}
		})
	}
}
