package alt_db

import (
	"alt/domain"
	"context"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The read-status writes had two ways of answering "there is no such feed"
// (capability catalog §4-5). UpdateFeedStatus resolved the feed with its own
// SELECT and read pgx.ErrNoRows; MarkArticleAsRead ran one
// INSERT ... SELECT and read RowsAffected() == 0.
//
// These tests pin the surviving one. They assert the absence of the SELECT as
// much as the presence of the upsert: pgxmock fails an unexpected call, so a
// reintroduced resolve query fails here rather than only showing up as an
// extra round trip in production. The window the SELECT opened — feed deleted
// between the check and the write, upsert silently affecting nothing, caller
// told it succeeded — is what the single statement closes.

// createTestUserContext builds a request context carrying an authenticated
// user, as the middleware does.
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

func TestUpdateFeedStatus_UpsertsInOneStatement(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
	}{
		{
			// The stored row may carry tracking parameters; normalisation on
			// this side is what makes the two match.
			name:     "url the caller stripped of utm parameters",
			inputURL: "https://example.com/article",
		},
		{
			name:     "url with a trailing slash",
			inputURL: "https://example.com/article/",
		},
		{
			name:     "long real-world url",
			inputURL: "https://www.nationalelfservice.net/treatment/complementary-and-alternative/from-pills-to-people-the-rise-of-social-prescribing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			repo := &FeedRepository{pool: mock}
			ctx, userUUID := createTestUserContext("550e8400-e29b-41d4-a716-446655440000")

			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(readStatusUpsertByWebsiteURLQuery)).
				WithArgs(pgxmock.AnyArg(), userUUID).
				WillReturnResult(pgxmock.NewResult("INSERT", 1))
			mock.ExpectCommit()

			parsedURL, err := url.Parse(tt.inputURL)
			require.NoError(t, err)

			require.NoError(t, repo.UpdateFeedStatus(ctx, *parsedURL, userUUID))
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpdateFeedStatus_NormalizesBeforeMatching(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	ctx, userUUID := createTestUserContext("550e8400-e29b-41d4-a716-446655440020")

	// The tracking parameters are stripped here, not in the SQL: the argument
	// the provider sends is the normalised URL, so two callers that spell the
	// same feed differently write one read_status row rather than two.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(readStatusUpsertByWebsiteURLQuery)).
		WithArgs("https://example.com/article", userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	parsedURL, err := url.Parse("https://example.com/article?utm_source=newsletter&utm_medium=email")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateFeedStatus(ctx, *parsedURL, userUUID))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_NoFeedForURLIsErrFeedNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	ctx, userUUID := createTestUserContext("550e8400-e29b-41d4-a716-446655440003")

	// Zero rows affected is how a missing feed presents: the INSERT's SELECT
	// matched nothing, so there was nothing to write and nothing to conflict
	// with. The transaction rolls back rather than committing a no-op.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(readStatusUpsertByWebsiteURLQuery)).
		WithArgs(pgxmock.AnyArg(), userUUID).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	mock.ExpectRollback()

	parsedURL, err := url.Parse("https://example.com/nonexistent")
	require.NoError(t, err)

	err = repo.UpdateFeedStatus(ctx, *parsedURL, userUUID)
	assert.ErrorIs(t, err, domain.ErrFeedNotFound, "expected the domain error")
	assert.NotErrorIs(t, err, pgx.ErrNoRows, "the database error must not reach the caller")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateFeedStatus_ExecFailureRollsBack(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	ctx, userUUID := createTestUserContext("550e8400-e29b-41d4-a716-446655440004")

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(readStatusUpsertByWebsiteURLQuery)).
		WithArgs(pgxmock.AnyArg(), userUUID).
		WillReturnError(pgx.ErrTxClosed)
	mock.ExpectRollback()

	parsedURL, err := url.Parse("https://example.com/article")
	require.NoError(t, err)

	err = repo.UpdateFeedStatus(ctx, *parsedURL, userUUID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrFeedNotFound,
		"a database failure is not the same answer as a URL nobody subscribes to")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMarkArticleAsRead_SharesTheFeedNotFoundSemantics is the other half of
// §4-5. The two procedures stay separate — they are different caller
// vocabularies over the same table — but a consumer that sees only a Connect
// code cannot tell which one answered, so the code has to mean the same thing.
func TestMarkArticleAsRead_SharesTheFeedNotFoundSemantics(t *testing.T) {
	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440005")

	tests := []struct {
		name       string
		rows       int64
		wantNoErr  bool
		inputURL   string
		wantArgURL string
	}{
		{
			name:       "feed exists",
			rows:       1,
			wantNoErr:  true,
			inputURL:   "https://example.com/post?utm_campaign=x",
			wantArgURL: "https://example.com/post",
		},
		{
			name:       "no feed for the url",
			rows:       0,
			inputURL:   "https://example.com/missing",
			wantArgURL: "https://example.com/missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			repo := &SubscriptionRepository{pool: mock}

			// Same query constant as UpdateFeedStatus. Sharing the string is
			// the point: "one semantics" that lived in two copies of the SQL
			// would be one edit away from being two again.
			mock.ExpectExec(regexp.QuoteMeta(readStatusUpsertByWebsiteURLQuery)).
				WithArgs(tt.wantArgURL, userID).
				WillReturnResult(pgxmock.NewResult("INSERT", tt.rows))

			parsedURL, err := url.Parse(tt.inputURL)
			require.NoError(t, err)

			err = repo.MarkArticleAsRead(context.Background(), *parsedURL, userID)
			if tt.wantNoErr {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, domain.ErrFeedNotFound)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
