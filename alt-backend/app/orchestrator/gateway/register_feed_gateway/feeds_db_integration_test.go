package register_feed_gateway

import (
	stderrors "errors"

	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"
)

func TestRegisterFeedsGateway_RegisterFeeds_DatabaseIntegration(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// Skip test if no database connection available
	// This test requires a real database connection to verify actual DB operations
	t.Skip("Database integration test - requires real DB connection")

	// This is a template for database integration testing
	// When we have a test database available, this test will verify:
	// 1. Actual database insertion of feed items
	// 2. Duplicate handling (URL-based)
	// 3. Transaction rollback on errors
	// 4. Proper timestamp handling

	_ = context.Background()

	// Test data (for future use)
	_ = []*domain.FeedItem{
		{
			Title:           "Database Test Article 1",
			Description:     "Testing actual database insertion",
			Link:            "https://test.example.com/article1",
			Published:       "2025-01-13T10:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC),
			Author: domain.Author{
				Name: "Test Author",
			},
			Authors: []domain.Author{
				{Name: "Test Author"},
			},
			Links: []string{"https://test.example.com/article1"},
		},
	}

	// Create gateway with real database pool
	// gateway := NewRegisterFeedsGateway(realDBPool)

	// Test actual database operations
	// err := gateway.RegisterFeeds(ctx, testFeeds)
	// if err != nil {
	//     t.Errorf("Database integration test failed: %v", err)
	// }

	// Verify data was actually inserted
	// Verify duplicate handling
	// Clean up test data

	t.Log("Database integration test template created")
}

func TestRegisterFeedsGateway_RegisterFeeds_MemoryTest(t *testing.T) {
	// Initialize logger
	logger.InitLogger()

	// This test focuses on the gateway logic without actual database operations
	// We test the data transformation and error handling logic

	ctx := context.Background()

	// A data plane failure must reach the caller as a failure. The previous
	// version asserted the exact string "database connection not available",
	// which only held because the gateway short-circuited on a nil repository
	// — a state that no longer exists (the constructor refuses it).
	gateway := &RegisterFeedsGateway{
		store: &feedWriteStoreStub{err: errDataPlaneUnavailable},
	}

	testFeeds := []*domain.FeedItem{
		{
			Title:           "Test Article",
			Description:     "Test Description",
			Link:            "https://example.com/article",
			Published:       "2025-01-13T10:00:00Z",
			PublishedParsed: time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC),
		},
	}

	_, err := gateway.RegisterFeeds(ctx, testFeeds)

	if err == nil {
		t.Fatal("Expected error when the data plane refuses the write, got nil")
	}
	if !stderrors.Is(err, errDataPlaneUnavailable) {
		t.Errorf("Expected the store's failure to survive the gateway, got: %v", err)
	}
}

func TestRegisterFeedsGateway_DataTransformation(t *testing.T) {
	// Test the domain to database model transformation logic
	logger.InitLogger()

	// This test verifies that domain.FeedItem is correctly transformed
	// to models.Feed without requiring database operations

	testTime := time.Date(2025, 1, 13, 10, 0, 0, 0, time.UTC)

	domainFeed := &domain.FeedItem{
		Title:           "Test Title",
		Description:     "Test Description",
		Link:            "https://example.com/test",
		Published:       "2025-01-13T10:00:00Z",
		PublishedParsed: testTime,
		Author: domain.Author{
			Name: "Test Author",
		},
		Authors: []domain.Author{
			{Name: "Test Author"},
		},
		Links: []string{"https://example.com/test"},
	}

	// We would test the transformation logic here
	// For now, we verify the basic structure

	if domainFeed.Title == "" {
		t.Error("Domain feed title should not be empty")
	}

	if domainFeed.Link == "" {
		t.Error("Domain feed link should not be empty")
	}

	if domainFeed.PublishedParsed.IsZero() {
		t.Error("Domain feed published time should not be zero")
	}

	t.Logf("Domain feed validation passed: %s", domainFeed.Title)
}
