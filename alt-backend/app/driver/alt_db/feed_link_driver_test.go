package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchFeedLinksWithAvailability_AllWithAvailability(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Now()
	reason := "connection timeout"

	rows := pgxmock.NewRows([]string{
		"id", "url", "is_active", "consecutive_failures", "last_failure_at", "last_failure_reason",
	}).
		AddRow(id1, "https://example.com/feed.xml", boolPtr(true), intPtr(0), nil, nil).
		AddRow(id2, "https://blog.example.org/rss", boolPtr(true), intPtr(3), &now, &reason)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT fl.id, fl.url, fla.is_active, fla.consecutive_failures, fla.last_failure_at, fla.last_failure_reason FROM feed_links fl LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id ORDER BY fl.url ASC`)).
		WillReturnRows(rows)

	links, err := repo.FetchFeedLinksWithAvailability(context.Background())
	require.NoError(t, err)
	assert.Len(t, links, 2)

	assert.Equal(t, id1, links[0].ID)
	assert.Equal(t, "https://example.com/feed.xml", links[0].URL)
	require.NotNil(t, links[0].Availability)
	assert.True(t, links[0].Availability.IsActive)
	assert.Equal(t, 0, links[0].Availability.ConsecutiveFailures)

	assert.Equal(t, id2, links[1].ID)
	require.NotNil(t, links[1].Availability)
	assert.True(t, links[1].Availability.IsActive)
	assert.Equal(t, 3, links[1].Availability.ConsecutiveFailures)
	assert.Equal(t, &reason, links[1].Availability.LastFailureReason)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchFeedLinksWithAvailability_WithNullAvailability(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	id1 := uuid.New()

	rows := pgxmock.NewRows([]string{
		"id", "url", "is_active", "consecutive_failures", "last_failure_at", "last_failure_reason",
	}).
		AddRow(id1, "https://example.com/feed.xml", nil, nil, nil, nil)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT fl.id, fl.url, fla.is_active, fla.consecutive_failures, fla.last_failure_at, fla.last_failure_reason FROM feed_links fl LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id ORDER BY fl.url ASC`)).
		WillReturnRows(rows)

	links, err := repo.FetchFeedLinksWithAvailability(context.Background())
	require.NoError(t, err)
	assert.Len(t, links, 1)

	assert.Equal(t, id1, links[0].ID)
	assert.Nil(t, links[0].Availability)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchFeedLinksWithAvailability_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT fl.id, fl.url, fla.is_active, fla.consecutive_failures, fla.last_failure_at, fla.last_failure_reason FROM feed_links fl LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id ORDER BY fl.url ASC`)).
		WillReturnError(errors.New("db connection error"))

	links, err := repo.FetchFeedLinksWithAvailability(context.Background())
	assert.Error(t, err)
	assert.Nil(t, links)

	require.NoError(t, mock.ExpectationsWereMet())
}

func boolPtr(b bool) *bool       { return &b }
func intPtr(i int) *int           { return &i }
