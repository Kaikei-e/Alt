package datahubapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// These cover what the Pact interactions cannot: the calls a well-behaved
// consumer never makes. For this batch that is where a tenant would leak (a
// scoped write with no user_id) and where the §4-5 unification would silently
// come apart (one procedure answering NotFound while its twin answers
// Internal for the same absence).

const (
	testUserID     = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	testFeedLinkID = "9c858901-8a57-4791-81fe-4c455b099bc9"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type fakeReadStatePort struct {
	err           error
	readFeedIDs   []uuid.UUID
	subscribedIDs []uuid.UUID
	subscriptions []*domain.FeedSource

	gotUserID     uuid.UUID
	gotURL        string
	gotFeedLinkID uuid.UUID
	gotFeedIDs    []uuid.UUID
	calls         int
}

func (f *fakeReadStatePort) MarkFeedRead(_ context.Context, feedURL string, userID uuid.UUID) error {
	f.calls++
	f.gotURL, f.gotUserID = feedURL, userID
	return f.err
}

func (f *fakeReadStatePort) MarkArticleRead(_ context.Context, articleURL string, userID uuid.UUID) error {
	f.calls++
	f.gotURL, f.gotUserID = articleURL, userID
	return f.err
}

func (f *fakeReadStatePort) ReadFeedIDs(_ context.Context, userID uuid.UUID, feedIDs []uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	f.gotUserID, f.gotFeedIDs = userID, feedIDs
	return f.readFeedIDs, f.err
}

func (f *fakeReadStatePort) AllReadFeedIDs(_ context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	f.gotUserID = userID
	return f.readFeedIDs, f.err
}

func (f *fakeReadStatePort) SubscribedFeedLinkIDs(_ context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	f.gotUserID = userID
	return f.subscribedIDs, f.err
}

func (f *fakeReadStatePort) ListSubscriptions(_ context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	f.calls++
	f.gotUserID = userID
	return f.subscriptions, f.err
}

func (f *fakeReadStatePort) Subscribe(_ context.Context, userID, feedLinkID uuid.UUID) error {
	f.calls++
	f.gotUserID, f.gotFeedLinkID = userID, feedLinkID
	return f.err
}

func (f *fakeReadStatePort) Unsubscribe(_ context.Context, userID, feedLinkID uuid.UUID) error {
	f.calls++
	f.gotUserID, f.gotFeedLinkID = userID, feedLinkID
	return f.err
}

func (f *fakeReadStatePort) AddFavorite(_ context.Context, feedURL string, userID uuid.UUID) error {
	f.calls++
	f.gotURL, f.gotUserID = feedURL, userID
	return f.err
}

func (f *fakeReadStatePort) RemoveFavorite(_ context.Context, feedURL string, userID uuid.UUID) error {
	f.calls++
	f.gotURL, f.gotUserID = feedURL, userID
	return f.err
}

type fakeTagReadPort struct {
	err          error
	tags         []*domain.FeedTag
	cooccurrence []*domain.TagCooccurrence
	hits         []domain.GlobalTagHit
	counts       []domain.TagArticleCount

	gotLimit  int
	gotCursor *time.Time
	gotSince  time.Time
	gotUserID uuid.UUID
	calls     int
}

func (f *fakeTagReadPort) ArticleTags(context.Context, string) ([]*domain.FeedTag, error) {
	f.calls++
	return f.tags, f.err
}

func (f *fakeTagReadPort) FeedTags(_ context.Context, _ string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	f.calls++
	f.gotCursor, f.gotLimit = cursor, limit
	return f.tags, f.err
}

func (f *fakeTagReadPort) Cooccurrences(context.Context, []string) ([]*domain.TagCooccurrence, error) {
	f.calls++
	return f.cooccurrence, f.err
}

func (f *fakeTagReadPort) SearchByPrefix(_ context.Context, _ string, limit int) ([]domain.GlobalTagHit, error) {
	f.calls++
	f.gotLimit = limit
	return f.hits, f.err
}

func (f *fakeTagReadPort) ArticleCounts(_ context.Context, userID uuid.UUID, since time.Time) ([]domain.TagArticleCount, error) {
	f.calls++
	f.gotUserID, f.gotSince = userID, since
	return f.counts, f.err
}

func newBatch4Handler(readState *fakeReadStatePort, tagRead *fakeTagReadPort) (*Handler, *fakeReadStatePort, *fakeTagReadPort) {
	if readState == nil {
		readState = &fakeReadStatePort{}
	}
	if tagRead == nil {
		tagRead = &fakeTagReadPort{}
	}
	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	WithWave3Batch4Capabilities(readState, tagRead)(h)
	return h, readState, tagRead
}

// ---------------------------------------------------------------------------
// §4-5: the two read-status writes answer absence identically
// ---------------------------------------------------------------------------

// TestReadStateWrites_ShareTheNotFoundSemantics is the handler half of the
// catalog §4-5 unification.
//
// The driver tests pin that both writes derive "no such feed" from one
// statement's RowsAffected(); this pins that both then report it as the same
// Connect code. Together they are the claim: a consumer that gets NotFound
// from either procedure learns the same thing, which is what let the
// SELECT-first implementation be retired rather than merely renamed.
//
// The favourites are in the same table because they raise the other absence
// sentinel — pgx.ErrNoRows rather than domain.ErrFeedNotFound — and a mapping
// that handled one and not the other would turn a missing feed into a 500.
func TestReadStateWrites_ShareTheNotFoundSemantics(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Handler, context.Context) error
	}{
		{
			name: "MarkFeedRead",
			invoke: func(h *Handler, ctx context.Context) error {
				_, err := h.MarkFeedRead(ctx, connect.NewRequest(&datahubv1.MarkFeedReadRequest{
					FeedUrl: "https://example.com/feed.xml", UserId: testUserID,
				}))
				return err
			},
		},
		{
			name: "MarkArticleRead",
			invoke: func(h *Handler, ctx context.Context) error {
				_, err := h.MarkArticleRead(ctx, connect.NewRequest(&datahubv1.MarkArticleReadRequest{
					ArticleUrl: "https://example.com/post", UserId: testUserID,
				}))
				return err
			},
		},
		{
			name: "AddFavoriteFeed",
			invoke: func(h *Handler, ctx context.Context) error {
				_, err := h.AddFavoriteFeed(ctx, connect.NewRequest(&datahubv1.AddFavoriteFeedRequest{
					FeedUrl: "https://example.com/feed.xml", UserId: testUserID,
				}))
				return err
			},
		},
		{
			name: "RemoveFavoriteFeed",
			invoke: func(h *Handler, ctx context.Context) error {
				_, err := h.RemoveFavoriteFeed(ctx, connect.NewRequest(&datahubv1.RemoveFavoriteFeedRequest{
					FeedUrl: "https://example.com/feed.xml", UserId: testUserID,
				}))
				return err
			},
		},
	}

	absences := []struct {
		name string
		err  error
	}{
		{name: "domain.ErrFeedNotFound", err: domain.ErrFeedNotFound},
		{name: "pgx.ErrNoRows", err: pgx.ErrNoRows},
	}

	for _, tt := range tests {
		for _, absence := range absences {
			t.Run(tt.name+"/"+absence.name, func(t *testing.T) {
				h, _, _ := newBatch4Handler(&fakeReadStatePort{err: absence.err}, nil)
				err := tt.invoke(h, context.Background())
				require.Error(t, err)
				assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err),
					"every §2.I write must report a URL naming nothing the same way")
			})
		}

		t.Run(tt.name+"/database failure is not NotFound", func(t *testing.T) {
			h, _, _ := newBatch4Handler(&fakeReadStatePort{err: errors.New("connection refused")}, nil)
			err := tt.invoke(h, context.Background())
			require.Error(t, err)
			assert.Equal(t, connect.CodeInternal, connect.CodeOf(err),
				"a fault must stay distinguishable from an absence, or retries chase a missing feed")
		})
	}
}

// ---------------------------------------------------------------------------
// Tenant scoping
// ---------------------------------------------------------------------------

// TestReadState_RequiresUserID refuses before touching the port.
//
// A zero UUID is a valid uuid.UUID, so a defaulted tenant would read and write
// the state of a user that does not exist: the call succeeds, nothing the
// caller can see changes, and there is no error anywhere to notice.
func TestReadState_RequiresUserID(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Handler, context.Context) error
	}{
		{"MarkFeedRead", func(h *Handler, ctx context.Context) error {
			_, err := h.MarkFeedRead(ctx, connect.NewRequest(&datahubv1.MarkFeedReadRequest{FeedUrl: "https://e.com/f"}))
			return err
		}},
		{"MarkArticleRead", func(h *Handler, ctx context.Context) error {
			_, err := h.MarkArticleRead(ctx, connect.NewRequest(&datahubv1.MarkArticleReadRequest{ArticleUrl: "https://e.com/a"}))
			return err
		}},
		{"GetReadFeedIDs", func(h *Handler, ctx context.Context) error {
			_, err := h.GetReadFeedIDs(ctx, connect.NewRequest(&datahubv1.GetReadFeedIDsRequest{FeedIds: []string{testFeedLinkID}}))
			return err
		}},
		{"GetAllReadFeedIDs", func(h *Handler, ctx context.Context) error {
			_, err := h.GetAllReadFeedIDs(ctx, connect.NewRequest(&datahubv1.GetAllReadFeedIDsRequest{}))
			return err
		}},
		{"GetUserSubscribedFeedLinkIDs", func(h *Handler, ctx context.Context) error {
			_, err := h.GetUserSubscribedFeedLinkIDs(ctx, connect.NewRequest(&datahubv1.GetUserSubscribedFeedLinkIDsRequest{}))
			return err
		}},
		{"ListSubscriptions", func(h *Handler, ctx context.Context) error {
			_, err := h.ListSubscriptions(ctx, connect.NewRequest(&datahubv1.ListSubscriptionsRequest{}))
			return err
		}},
		{"Subscribe", func(h *Handler, ctx context.Context) error {
			_, err := h.Subscribe(ctx, connect.NewRequest(&datahubv1.SubscribeRequest{FeedLinkId: testFeedLinkID}))
			return err
		}},
		{"Unsubscribe", func(h *Handler, ctx context.Context) error {
			_, err := h.Unsubscribe(ctx, connect.NewRequest(&datahubv1.UnsubscribeRequest{FeedLinkId: testFeedLinkID}))
			return err
		}},
		{"AddFavoriteFeed", func(h *Handler, ctx context.Context) error {
			_, err := h.AddFavoriteFeed(ctx, connect.NewRequest(&datahubv1.AddFavoriteFeedRequest{FeedUrl: "https://e.com/f"}))
			return err
		}},
		{"RemoveFavoriteFeed", func(h *Handler, ctx context.Context) error {
			_, err := h.RemoveFavoriteFeed(ctx, connect.NewRequest(&datahubv1.RemoveFavoriteFeedRequest{FeedUrl: "https://e.com/f"}))
			return err
		}},
		{"GetTagArticleCounts", func(h *Handler, ctx context.Context) error {
			_, err := h.GetTagArticleCounts(ctx, connect.NewRequest(&datahubv1.GetTagArticleCountsRequest{
				Since: timestamppb.New(time.Now().Add(-24 * time.Hour)),
			}))
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, readState, tagRead := newBatch4Handler(nil, nil)
			err := tt.invoke(h, context.Background())
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Zero(t, readState.calls+tagRead.calls, "the query must not run without a tenant")
		})
	}
}

func TestReadState_RejectsMalformedUserID(t *testing.T) {
	h, readState, _ := newBatch4Handler(nil, nil)

	_, err := h.GetAllReadFeedIDs(context.Background(),
		connect.NewRequest(&datahubv1.GetAllReadFeedIDsRequest{UserId: "not-a-uuid"}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, readState.calls)
}

// ---------------------------------------------------------------------------
// Read-state reads
// ---------------------------------------------------------------------------

func TestGetReadFeedIDs_EmptyBatchSkipsTheQuery(t *testing.T) {
	h, readState, _ := newBatch4Handler(nil, nil)

	resp, err := h.GetReadFeedIDs(context.Background(),
		connect.NewRequest(&datahubv1.GetReadFeedIDsRequest{UserId: testUserID}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetReadFeedIds())
	assert.Zero(t, readState.calls, "an empty page has an empty overlay without asking the database")
}

func TestGetReadFeedIDs_RejectsUnparseableFeedID(t *testing.T) {
	h, readState, _ := newBatch4Handler(nil, nil)

	_, err := h.GetReadFeedIDs(context.Background(),
		connect.NewRequest(&datahubv1.GetReadFeedIDsRequest{
			UserId:  testUserID,
			FeedIds: []string{testFeedLinkID, "not-a-uuid"},
		}))

	// Rejecting the batch rather than dropping the bad entry: a dropped id
	// comes back as "unread", which is a wrong answer rather than a missing
	// one, and the reader sees an article they had already marked reappear.
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, readState.calls)
}

func TestGetReadFeedIDs_RefusesOversizedBatch(t *testing.T) {
	ids := make([]string, maxReadFeedIDsPerRequest+1)
	for i := range ids {
		ids[i] = uuid.New().String()
	}

	h, readState, _ := newBatch4Handler(nil, nil)
	_, err := h.GetReadFeedIDs(context.Background(),
		connect.NewRequest(&datahubv1.GetReadFeedIDsRequest{UserId: testUserID, FeedIds: ids}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, readState.calls)
}

// TestListSubscriptions_OmitsSubscribedAtForUnfollowedLinks pins the one place
// this response is not a straight copy of the row.
//
// The query coalesces the missing subscription join to now(), so an
// unfollowed link carries a timestamp that means nothing. Forwarding it would
// have the screen show a "following since" date for something nobody follows.
func TestListSubscriptions_OmitsSubscribedAtForUnfollowedLinks(t *testing.T) {
	subscribedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	h, _, _ := newBatch4Handler(&fakeReadStatePort{
		subscriptions: []*domain.FeedSource{
			{ID: testFeedLinkID, URL: "https://example.com/feed.xml", IsSubscribed: true, CreatedAt: subscribedAt},
			{ID: "1f0b1a1d-0000-4000-8000-000000000001", URL: "https://other.example.com/feed.xml", IsSubscribed: false, CreatedAt: time.Now()},
		},
	}, nil)

	resp, err := h.ListSubscriptions(context.Background(),
		connect.NewRequest(&datahubv1.ListSubscriptionsRequest{UserId: testUserID}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.GetSubscriptions(), 2)

	followed := resp.Msg.GetSubscriptions()[0]
	assert.True(t, followed.GetIsSubscribed())
	require.NotNil(t, followed.SubscribedAt)
	assert.Equal(t, subscribedAt, followed.GetSubscribedAt().AsTime())

	unfollowed := resp.Msg.GetSubscriptions()[1]
	assert.False(t, unfollowed.GetIsSubscribed())
	assert.Nil(t, unfollowed.SubscribedAt, "a coalesced now() is not a subscription date")
}

// ---------------------------------------------------------------------------
// Tag reads
// ---------------------------------------------------------------------------

// TestGetArticleTags_UntaggedIsEmptyNotNotFound pins the answer the on-the-fly
// generation path depends on. If an untagged article were NotFound, the caller
// would treat it as a fault and never ask mq-hub to generate anything.
func TestGetArticleTags_UntaggedIsEmptyNotNotFound(t *testing.T) {
	h, _, _ := newBatch4Handler(nil, &fakeTagReadPort{tags: nil})

	resp, err := h.GetArticleTags(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleTagsRequest{ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetTags())
}

// TestFeedTagsToProto_LeavesUnsetTimestampsAbsent guards the shape of the row
// the article-tag read produces: it selects neither feed_id nor updated_at, so
// a zero time must not become 1970 on the wire — a consumer would sort on it.
func TestFeedTagsToProto_LeavesUnsetTimestampsAbsent(t *testing.T) {
	created := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	h, _, _ := newBatch4Handler(nil, &fakeTagReadPort{
		tags: []*domain.FeedTag{{ID: "tag-1", TagName: "AI", CreatedAt: created}},
	})

	resp, err := h.GetArticleTags(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleTagsRequest{ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.GetTags(), 1)
	tag := resp.Msg.GetTags()[0]
	assert.Equal(t, "AI", tag.GetTagName())
	assert.Equal(t, created, tag.GetCreatedAt().AsTime())
	assert.Nil(t, tag.UpdatedAt, "a column the query never selects must not arrive as an epoch")
	assert.Empty(t, tag.GetFeedId())
}

func TestGetFeedTags_LimitAndCursor(t *testing.T) {
	cursor := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		limit      int32
		cursor     *timestamppb.Timestamp
		wantLimit  int
		wantCursor bool
	}{
		{name: "unset limit gets the default", limit: 0, wantLimit: defaultTagLimit},
		{name: "limit within the ceiling passes through", limit: 25, wantLimit: 25},
		{name: "limit above the ceiling is clamped", limit: maxTagLimit + 1, wantLimit: maxTagLimit},
		{name: "cursor is forwarded", limit: 10, cursor: timestamppb.New(cursor), wantLimit: 10, wantCursor: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, tagRead := newBatch4Handler(nil, nil)

			_, err := h.GetFeedTags(context.Background(),
				connect.NewRequest(&datahubv1.GetFeedTagsRequest{
					FeedId: testFeedLinkID, Limit: tt.limit, Cursor: tt.cursor,
				}))

			require.NoError(t, err)
			assert.Equal(t, tt.wantLimit, tagRead.gotLimit)
			if tt.wantCursor {
				require.NotNil(t, tagRead.gotCursor)
				assert.Equal(t, cursor, *tagRead.gotCursor)
			} else {
				assert.Nil(t, tagRead.gotCursor, "no cursor means the first page, not the epoch")
			}
		})
	}
}

func TestGetTagCooccurrences_EmptyInputSkipsTheQuery(t *testing.T) {
	h, _, tagRead := newBatch4Handler(nil, nil)

	resp, err := h.GetTagCooccurrences(context.Background(),
		connect.NewRequest(&datahubv1.GetTagCooccurrencesRequest{}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetCooccurrences())
	assert.Zero(t, tagRead.calls, "no tags means no pairs, without a self-join over article_tags")
}

// TestGetTagArticleCounts_RequiresSince refuses the unbounded count.
//
// Defaulting `since` would answer a different and far more expensive question
// than the caller asked — the user's entire history rather than a window — and
// would do it silently, as a slow success.
func TestGetTagArticleCounts_RequiresSince(t *testing.T) {
	h, _, tagRead := newBatch4Handler(nil, nil)

	_, err := h.GetTagArticleCounts(context.Background(),
		connect.NewRequest(&datahubv1.GetTagArticleCountsRequest{UserId: testUserID}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, tagRead.calls)
}

func TestGetTagArticleCounts_ForwardsWindowAndTenant(t *testing.T) {
	since := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	h, _, tagRead := newBatch4Handler(nil, &fakeTagReadPort{
		counts: []domain.TagArticleCount{{TagName: "AI", ArticleCount: 10}},
	})

	resp, err := h.GetTagArticleCounts(context.Background(),
		connect.NewRequest(&datahubv1.GetTagArticleCountsRequest{
			UserId: testUserID, Since: timestamppb.New(since),
		}))

	require.NoError(t, err)
	assert.Equal(t, uuid.MustParse(testUserID), tagRead.gotUserID)
	assert.Equal(t, since, tagRead.gotSince)
	require.Len(t, resp.Msg.GetCounts(), 1)
	assert.Equal(t, "AI", resp.Msg.GetCounts()[0].GetTagName())
	assert.Equal(t, int32(10), resp.Msg.GetCounts()[0].GetArticleCount())
}

// TestWithWave3Batch4Capabilities_PanicsOnNil is CLAUDE.md rule 8: an unwired
// port must be a process that does not start, not a procedure that answers
// Unimplemented — which is also what a retired procedure answers.
func TestWithWave3Batch4Capabilities_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() { WithWave3Batch4Capabilities(nil, &fakeTagReadPort{}) })
	assert.Panics(t, func() { WithWave3Batch4Capabilities(&fakeReadStatePort{}, nil) })
}
