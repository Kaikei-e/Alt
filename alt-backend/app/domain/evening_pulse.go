package domain

import (
	"errors"
	"time"
)

// PulseStatus represents the status of Evening Pulse generation
type PulseStatus string

const (
	PulseStatusNormal   PulseStatus = "normal"
	PulseStatusPartial  PulseStatus = "partial"
	PulseStatusQuietDay PulseStatus = "quiet_day"
	PulseStatusError    PulseStatus = "error"
)

// TopicRole represents the role of a topic in Evening Pulse
type TopicRole string

const (
	TopicRoleNeedToKnow  TopicRole = "need_to_know"
	TopicRoleTrend       TopicRole = "trend"
	TopicRoleSerendipity TopicRole = "serendipity"
)

// Confidence represents the confidence level of a selection
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// PulseRationale explains why a topic was selected
type PulseRationale struct {
	Text       string     `json:"text"`
	Confidence Confidence `json:"confidence"`
}

// RepresentativeArticle represents a key article for display in topic cards
type RepresentativeArticle struct {
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	SourceURL   string `json:"source_url"`
	SourceName  string `json:"source_name"`
	PublishedAt string `json:"published_at"`
}

// PulseTopic represents a selected topic for Evening Pulse
type PulseTopic struct {
	ClusterID              int64                   `json:"cluster_id"`
	Role                   TopicRole               `json:"role"`
	Title                  string                  `json:"title"`
	Rationale              PulseRationale          `json:"rationale"`
	ArticleCount           int                     `json:"article_count"`
	SourceCount            int                     `json:"source_count"`
	Tier1Count             *int                    `json:"tier1_count,omitempty"`
	TimeAgo                string                  `json:"time_ago"`
	TrendMultiplier        *float64                `json:"trend_multiplier,omitempty"`
	Genre                  *string                 `json:"genre,omitempty"`
	ArticleIDs             []string                `json:"article_ids"`
	RepresentativeArticles []RepresentativeArticle `json:"representative_articles"`
	TopEntities            []string                `json:"top_entities"`
	SourceNames            []string                `json:"source_names"`
}

// WeeklyHighlight represents a notable topic from the past week
type WeeklyHighlight struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
	Role  string `json:"role"`
}

// QuietDayInfo provides fallback content when no topics are available
type QuietDayInfo struct {
	Message          string            `json:"message"`
	WeeklyHighlights []WeeklyHighlight `json:"weekly_highlights"`
}

// EveningPulse represents the Evening Pulse data
type EveningPulse struct {
	JobID       string       `json:"job_id"`
	Date        string       `json:"date"`
	GeneratedAt time.Time    `json:"generated_at"`
	Status      PulseStatus  `json:"status"`
	Topics      []PulseTopic `json:"topics"`
	QuietDay    *QuietDayInfo `json:"quiet_day,omitempty"`
}

// ErrEveningPulseNotFound indicates that no Evening Pulse data was found
var ErrEveningPulseNotFound = errors.New("evening pulse not found")
