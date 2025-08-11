package testutil

import (
	"alt/domain"
	"alt/driver/models"
	"context"
	"errors"
	"time"
)

// Common test data generators
func CreateMockFeedItems() []*domain.FeedItem {
	return []*domain.FeedItem{
		{
			Title:       "Test Feed 1",
			Description: "Test Description 1",
			Link:        "https://test.com/feed1",
		},
		{
			Title:       "Test Feed 2",
			Description: "Test Description 2",
			Link:        "https://test.com/feed2",
		},
	}
}

func CreateSingleMockFeedItem() *domain.FeedItem {
	return &domain.FeedItem{
		Title:       "Single Test Feed",
		Description: "Single Test Description",
		Link:        "https://test.com/single-feed",
	}
}

func CreateEmptyFeedItems() []*domain.FeedItem {
	return []*domain.FeedItem{}
}

// Common error instances
var (
	ErrMockDatabase   = errors.New("mock database error")
	ErrMockNetwork    = errors.New("mock network error")
	ErrMockValidation = errors.New("mock validation error")
	ErrMockTimeout    = errors.New("mock timeout error")
)

// Context utilities
func CreateCancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// Test case generators for common scenarios
type TestCase struct {
	Name    string
	WantErr bool
}

func CommonErrorTestCases() []TestCase {
	return []TestCase{
		{
			Name:    "database error",
			WantErr: true,
		},
		{
			Name:    "network error",
			WantErr: true,
		},
		{
			Name:    "cancelled context",
			WantErr: true,
		},
	}
}

func CommonSuccessTestCases() []TestCase {
	return []TestCase{
		{
			Name:    "success",
			WantErr: false,
		},
	}
}

// Models.Feed test data generators
func CreateMockFeeds(count int) []*models.Feed {
	now := time.Now()
	feeds := make([]*models.Feed, count)
	
	for i := 0; i < count; i++ {
		feeds[i] = &models.Feed{
			ID:          "test-feed-id-" + string(rune('0'+i)),
			Title:       "Test Feed " + string(rune('0'+i+1)),
			Description: "Test Description " + string(rune('0'+i+1)),
			Link:        "https://test.com/feed" + string(rune('0'+i+1)),
			PubDate:     now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}
	
	return feeds
}

func CreateEmptyFeeds() []*models.Feed {
	return []*models.Feed{}
}
