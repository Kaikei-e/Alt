package alt_db

import (
	"alt/domain"
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test user context
func createTestUserContext(userID string) (context.Context, uuid.UUID) {
	userUUID, _ := uuid.Parse(userID)
	userCtx := &domain.UserContext{
		UserID:    userUUID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	return domain.SetUserContext(context.Background(), userCtx), userUUID
}

func TestUpdateFeedStatus_WithUTMParameters(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440000"
	testFeedID := "feed-123"

	// Database has URL with UTM parameters
	dbFeedURL := "https://example.com/article?utm_source=rss&utm_campaign=test"
	// Frontend sends URL without UTM parameters
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query that fetches all feeds
	rows := pgxmock.NewRows([]string{"id", "link"}).
		AddRow(testFeedID, dbFeedURL)

	mock.ExpectQuery("SELECT id, link FROM feeds").
		WillReturnRows(rows)

	// Mock the transaction
	mock.ExpectBegin()

	// Mock the INSERT/UPDATE query
	mock.ExpectExec("INSERT INTO read_status").
		WithArgs(testFeedID, userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectCommit()

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_WithTrailingSlash(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440001"
	testFeedID := "feed-123"

	// Database has URL with trailing slash
	dbFeedURL := "https://example.com/article/"
	// Frontend sends URL without trailing slash
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query that fetches all feeds
	rows := pgxmock.NewRows([]string{"id", "link"}).
		AddRow(testFeedID, dbFeedURL)

	mock.ExpectQuery("SELECT id, link FROM feeds").
		WillReturnRows(rows)

	// Mock the transaction
	mock.ExpectBegin()

	// Mock the INSERT/UPDATE query
	mock.ExpectExec("INSERT INTO read_status").
		WithArgs(testFeedID, userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectCommit()

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_RealWorldExample(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440002"
	testFeedID := "feed-123"

	// Real-world example from the logs
	dbFeedURL := "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing/?utm_source=rss&utm_medium=rss&utm_campaign=from-pills-to-people-the-rise-of-social-prescribing"
	inputFeedURL := "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query that fetches all feeds
	rows := pgxmock.NewRows([]string{"id", "link"}).
		AddRow(testFeedID, dbFeedURL)

	mock.ExpectQuery("SELECT id, link FROM feeds").
		WillReturnRows(rows)

	// Mock the transaction
	mock.ExpectBegin()

	// Mock the INSERT/UPDATE query
	mock.ExpectExec("INSERT INTO read_status").
		WithArgs(testFeedID, userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectCommit()

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_FeedNotFound(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440003"
	inputFeedURL := "https://example.com/nonexistent"

	// Create context with user
	ctx, _ := createTestUserContext(testUserID)

	// Mock the SELECT query that returns no rows
	rows := pgxmock.NewRows([]string{"id", "link"})

	mock.ExpectQuery("SELECT id, link FROM feeds").
		WillReturnRows(rows)

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert - should return pgx.ErrNoRows
	assert.ErrorIs(t, err, pgx.ErrNoRows)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_NoUserInContext(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Create context WITHOUT user
	ctx := context.Background()

	// Parse input URL
	parsedURL, err := url.Parse("https://example.com/article")
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert - should return authentication error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication required")
}

func TestUpdateFeedStatus_MultipleFeeds(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440004"
	testFeedID := "feed-456"

	// Database has multiple feeds, one matches after normalization
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query that returns multiple feeds
	rows := pgxmock.NewRows([]string{"id", "link"}).
		AddRow("feed-123", "https://other-example.com/article?utm_source=rss").
		AddRow(testFeedID, "https://example.com/article/?utm_campaign=test").
		AddRow("feed-789", "https://another-example.com/feed")

	mock.ExpectQuery("SELECT id, link FROM feeds").
		WillReturnRows(rows)

	// Mock the transaction
	mock.ExpectBegin()

	// Mock the INSERT/UPDATE query
	mock.ExpectExec("INSERT INTO read_status").
		WithArgs(testFeedID, userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectCommit()

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
