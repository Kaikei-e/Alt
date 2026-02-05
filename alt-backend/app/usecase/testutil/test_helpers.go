package testutil

import (
	"alt/domain"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Common test data generators
func CreateMockFeedItems() []*domain.FeedItem {
	baseTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	return []*domain.FeedItem{
		{
			Title:           "Test Feed 1",
			Description:     "Test Description 1",
			Link:            "https://test.com/feed1",
			Published:       baseTime.Format(time.RFC3339),
			PublishedParsed: baseTime,
		},
		{
			Title:           "Test Feed 2",
			Description:     "Test Description 2",
			Link:            "https://test.com/feed2",
			Published:       baseTime.Add(-1 * time.Hour).Format(time.RFC3339),
			PublishedParsed: baseTime.Add(-1 * time.Hour),
		},
	}
}

func CreateSingleMockFeedItem() *domain.FeedItem {
	now := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	return &domain.FeedItem{
		Title:           "Single Test Feed",
		Description:     "Single Test Description",
		Link:            "https://test.com/single-feed",
		Published:       now.Format(time.RFC3339),
		PublishedParsed: now,
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

// Domain Feed test data generators
func CreateMockDomainFeeds(count int) []*domain.Feed {
	now := time.Now()
	feeds := make([]*domain.Feed, count)

	for i := 0; i < count; i++ {
		feeds[i] = &domain.Feed{
			ID:          uuid.New(),
			Title:       "Test Feed " + string(rune('0'+i+1)),
			Description: "Test Description " + string(rune('0'+i+1)),
			Link:        "https://test.com/feed" + string(rune('0'+i+1)),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	return feeds
}

func CreateEmptyDomainFeeds() []*domain.Feed {
	return []*domain.Feed{}
}
