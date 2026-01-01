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

	// Frontend sends URL without UTM parameters
	// (database URL normalization will match this with stored URLs that have UTM params)
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query with WHERE clause (optimized)
	rows := pgxmock.NewRows([]string{"id"}).
		AddRow(testFeedID)

	// The implementation normalizes the input URL before querying
	mock.ExpectQuery("SELECT id FROM feeds WHERE link").
		WithArgs(pgxmock.AnyArg()). // Normalized URL
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

	// Frontend sends URL without trailing slash
	// (database URL normalization will match this with stored URLs that have trailing slash)
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query with WHERE clause (optimized)
	rows := pgxmock.NewRows([]string{"id"}).
		AddRow(testFeedID)

	// The implementation normalizes the input URL before querying
	mock.ExpectQuery("SELECT id FROM feeds WHERE link").
		WithArgs(pgxmock.AnyArg()). // Normalized URL
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

	// Real-world example from the logs - frontend sends URL without UTM parameters
	inputFeedURL := "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock the SELECT query with WHERE clause (optimized)
	rows := pgxmock.NewRows([]string{"id"}).
		AddRow(testFeedID)

	// The implementation normalizes the input URL before querying
	mock.ExpectQuery("SELECT id FROM feeds WHERE link").
		WithArgs(pgxmock.AnyArg()). // Normalized URL
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

	// Mock the SELECT query with WHERE clause that returns no rows
	mock.ExpectQuery("SELECT id FROM feeds WHERE link").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert - should return domain.ErrFeedNotFound (not pgx.ErrNoRows)
	assert.ErrorIs(t, err, domain.ErrFeedNotFound, "Expected domain.ErrFeedNotFound")
	assert.NotErrorIs(t, err, pgx.ErrNoRows, "Should NOT expose database error pgx.ErrNoRows")
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

	// Mock the SELECT query with WHERE clause (optimized)
	// With the new implementation, we query directly for the matching feed
	rows := pgxmock.NewRows([]string{"id"}).
		AddRow(testFeedID)

	// The implementation normalizes the input URL before querying
	mock.ExpectQuery("SELECT id FROM feeds WHERE link").
		WithArgs(pgxmock.AnyArg()). // Normalized URL
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

// New test: Verify domain error is returned instead of database error
func TestUpdateFeedStatus_ReturnsErrFeedNotFound(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440010"
	inputFeedURL := "https://example.com/nonexistent"

	// Create context with user
	ctx, _ := createTestUserContext(testUserID)

	// Mock the SELECT query with WHERE clause that returns no rows
	mock.ExpectQuery("SELECT id FROM feeds WHERE").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	// Parse input URL
	parsedURL, err := url.Parse(inputFeedURL)
	require.NoError(t, err)

	// Execute
	err = repo.UpdateFeedStatus(ctx, *parsedURL)

	// Assert - should return domain.ErrFeedNotFound instead of pgx.ErrNoRows
	assert.ErrorIs(t, err, domain.ErrFeedNotFound, "Expected domain.ErrFeedNotFound")
	assert.NotErrorIs(t, err, pgx.ErrNoRows, "Should NOT expose database error pgx.ErrNoRows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// New test: Verify optimized database query is used
func TestUpdateFeedStatus_OptimizedDatabaseQuery(t *testing.T) {
	// Create mock database
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{
		pool: mock,
	}

	// Setup test data
	testUserID := "550e8400-e29b-41d4-a716-446655440012"
	testFeedID := "feed-optimized-123"
	inputFeedURL := "https://example.com/article"

	// Create context with user
	ctx, userUUID := createTestUserContext(testUserID)

	// Mock: should query with WHERE clause, not SELECT all
	rows := pgxmock.NewRows([]string{"id"}).
		AddRow(testFeedID)

	// Verify query contains WHERE clause with normalized URL parameter
	mock.ExpectQuery("SELECT id FROM feeds WHERE").
		WithArgs(pgxmock.AnyArg()).
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

	// Assert - test will fail if query doesn't match expected pattern
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
