package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFeedID_Validation(t *testing.T) {
	t.Run("should return error when db is nil", func(t *testing.T) {
		feedID, err := GetFeedID(context.Background(), nil, "https://example.com/feed")

		assert.Error(t, err, "should return error when db is nil")
		assert.Empty(t, feedID, "feedID should be empty when db is nil")
	})
}

// TestGetFeedID_NotFound tests that GetFeedID returns ("", nil) when feed is not found
// This is a behavioral specification test - when a feed URL doesn't exist in the database,
// GetFeedID should return an empty string without an error, allowing callers to handle
// the "not found" case gracefully.
//
// Note: This test documents the expected behavior. Integration tests with a real database
// would be needed to fully verify the pgx.ErrNoRows handling.
func TestGetFeedID_NotFound_Behavior(t *testing.T) {
	t.Run("should return empty string without error when feed not found", func(t *testing.T) {
		// This test documents the expected behavior after the fix:
		// When pgx.ErrNoRows is returned (feed not found), GetFeedID should return ("", nil)
		// rather than ("", error) so callers can distinguish "not found" from actual errors.
		//
		// The actual verification requires integration tests with pgxmock or a real database.
		// This test serves as documentation of the expected contract.
		t.Skip("Requires database mock - see integration tests")
	})
}
