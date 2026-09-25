package datahub_capability_gateway

import (
	"reflect"
	"testing"
	"time"

	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

func TestFeedsToDriverFeeds(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	feedLinkIDStr := uuid.New().String()
	ogURL := "https://example.com/og.png"

	tests := []struct {
		name  string
		feeds []domain.FeedRegistration
		want  []models.Feed
	}{
		{
			name:  "empty feeds",
			feeds: []domain.FeedRegistration{},
			want:  []models.Feed{},
		},
		{
			name: "single feed registration",
			feeds: []domain.FeedRegistration{
				{
					Title:       "Tech News",
					Description: "Daily tech news",
					WebsiteURL:  "https://example.com/feed",
					PubDate:     now,
					CreatedAt:   now,
					UpdatedAt:   now,
					FeedLinkID:  &feedLinkIDStr,
					OgImageURL:  &ogURL,
				},
			},
			want: []models.Feed{
				{
					Title:       "Tech News",
					Description: "Daily tech news",
					WebsiteURL:  "https://example.com/feed",
					PubDate:     now,
					CreatedAt:   now,
					UpdatedAt:   now,
					FeedLinkID:  &feedLinkIDStr,
					OgImageURL:  &ogURL,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feedsToDriverFeeds(tt.feeds)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("feedsToDriverFeeds() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFeedRegistrationResultsFromDriver(t *testing.T) {
	tests := []struct {
		name string
		rows []alt_db.FeedRegistrationResult
		want []domain.FeedRegistrationResult
	}{
		{
			name: "empty results",
			rows: []alt_db.FeedRegistrationResult{},
			want: []domain.FeedRegistrationResult{},
		},
		{
			name: "mapped results",
			rows: []alt_db.FeedRegistrationResult{
				{FeedID: "feed-1", Created: true},
				{FeedID: "feed-2", Created: false},
			},
			want: []domain.FeedRegistrationResult{
				{FeedID: "feed-1", Created: true},
				{FeedID: "feed-2", Created: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feedRegistrationResultsFromDriver(tt.rows)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("feedRegistrationResultsFromDriver() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFeedRowsFromPageRows(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	feedID := uuid.New()
	articleID := "art-1"
	ogURL := "https://example.com/og.png"

	tests := []struct {
		name string
		rows []*alt_db.FeedPageRow
		want []*domain.FeedRow
	}{
		{
			name: "empty rows",
			rows: []*alt_db.FeedPageRow{},
			want: []*domain.FeedRow{},
		},
		{
			name: "single page row",
			rows: []*alt_db.FeedPageRow{
				{
					FeedID:      feedID,
					Title:       "Post 1",
					Description: "Desc 1",
					Link:        "https://example.com/post-1",
					PubDate:     now,
					CreatedAt:   now,
					UpdatedAt:   now,
					ArticleID:   &articleID,
					OgImageURL:  &ogURL,
				},
			},
			want: []*domain.FeedRow{
				{
					ID:          feedID.String(),
					Title:       "Post 1",
					Description: "Desc 1",
					WebsiteURL:  "https://example.com/post-1",
					PubDate:     now,
					CreatedAt:   now,
					UpdatedAt:   now,
					ArticleID:   &articleID,
					OgImageURL:  &ogURL,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feedRowsFromPageRows(tt.rows)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("feedRowsFromPageRows() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFeedRowsFromItems(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		items []*domain.FeedItem
		want  []*domain.FeedRow
	}{
		{
			name:  "empty items",
			items: []*domain.FeedItem{},
			want:  []*domain.FeedRow{},
		},
		{
			name: "single search feed item",
			items: []*domain.FeedItem{
				{
					Title:           "Search Result 1",
					Description:     "Snippet 1",
					Link:            "https://example.com/1",
					PublishedParsed: now,
				},
			},
			want: []*domain.FeedRow{
				{
					Title:       "Search Result 1",
					Description: "Snippet 1",
					WebsiteURL:  "https://example.com/1",
					PubDate:     now,
					CreatedAt:   now,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feedRowsFromItems(tt.items)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("feedRowsFromItems() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInoreaderSummariesFromDriver(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	author := "Author 1"

	tests := []struct {
		name string
		rows []*models.InoreaderSummary
		want []*domain.InoreaderSummary
	}{
		{
			name: "empty summaries",
			rows: []*models.InoreaderSummary{},
			want: []*domain.InoreaderSummary{},
		},
		{
			name: "single inoreader summary",
			rows: []*models.InoreaderSummary{
				{
					ArticleURL:  "https://example.com/article",
					Title:       "Title 1",
					Author:      &author,
					Content:     "Full content",
					ContentType: "html",
					PublishedAt: now,
					FetchedAt:   now,
					InoreaderID: "ino-123",
				},
			},
			want: []*domain.InoreaderSummary{
				{
					ArticleURL:  "https://example.com/article",
					Title:       "Title 1",
					Author:      &author,
					Content:     "Full content",
					ContentType: "html",
					PublishedAt: now,
					FetchedAt:   now,
					InoreaderID: "ino-123",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inoreaderSummariesFromDriver(tt.rows)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("inoreaderSummariesFromDriver() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFeedRowFromModel(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	feedLinkIDStr := uuid.New().String()
	articleID := "art-1"
	ogURL := "https://example.com/og.png"

	tests := []struct {
		name  string
		model *models.Feed
		want  *domain.FeedRow
	}{
		{
			name:  "nil model returns nil",
			model: nil,
			want:  nil,
		},
		{
			name: "valid model mapped correctly",
			model: &models.Feed{
				ID:          "feed-1",
				Title:       "Feed Title",
				Description: "Feed Description",
				WebsiteURL:  "https://example.com",
				PubDate:     now,
				CreatedAt:   now,
				UpdatedAt:   now,
				ArticleID:   &articleID,
				IsRead:      true,
				FeedLinkID:  &feedLinkIDStr,
				OgImageURL:  &ogURL,
			},
			want: &domain.FeedRow{
				ID:          "feed-1",
				Title:       "Feed Title",
				Description: "Feed Description",
				WebsiteURL:  "https://example.com",
				PubDate:     now,
				CreatedAt:   now,
				UpdatedAt:   now,
				ArticleID:   &articleID,
				IsRead:      true,
				FeedLinkID:  &feedLinkIDStr,
				OgImageURL:  &ogURL,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feedRowFromModel(tt.model)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("feedRowFromModel() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
