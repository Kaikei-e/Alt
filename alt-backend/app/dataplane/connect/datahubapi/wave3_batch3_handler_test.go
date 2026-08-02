package datahubapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cover what the Pact interactions cannot: the request the consumer
// never sends. A pact records an agreed exchange, so it can pin what a
// well-formed call gets back but says nothing about what a malformed one does
// — and for this batch the malformed cases are where the tenant leaks
// (a missing user_id on a scoped list) and where a wrong answer looks like a
// right one (an unset scope defaulting to ALL).

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type fakeFeedLinkPort struct {
	alreadyExisted bool
	links          []*domain.FeedLink
	withHealth     []*domain.FeedLinkWithHealth
	resolvedID     *string
	err            error
	gotURL         string
}

func (f *fakeFeedLinkPort) Register(_ context.Context, url string) (bool, error) {
	f.gotURL = url
	return f.alreadyExisted, f.err
}

func (f *fakeFeedLinkPort) BulkRegister(_ context.Context, urls []string) (int, int, []string, error) {
	return len(urls), 0, nil, f.err
}

func (f *fakeFeedLinkPort) List(context.Context) ([]*domain.FeedLink, error) {
	return f.links, f.err
}

func (f *fakeFeedLinkPort) ListWithHealth(context.Context) ([]*domain.FeedLinkWithHealth, error) {
	return f.withHealth, f.err
}

func (f *fakeFeedLinkPort) Delete(context.Context, uuid.UUID) error { return f.err }

func (f *fakeFeedLinkPort) ResolveIDByURL(_ context.Context, feedURL string) (*string, error) {
	f.gotURL = feedURL
	return f.resolvedID, f.err
}

func (f *fakeFeedLinkPort) ListDomains(context.Context) ([]domain.FeedLinkDomain, error) {
	return nil, f.err
}

func (f *fakeFeedLinkPort) ListPollable(context.Context) ([]domain.FeedLink, error) {
	return nil, f.err
}

func (f *fakeFeedLinkPort) ListForExport(context.Context) ([]*domain.FeedLinkForExport, error) {
	return nil, f.err
}

type fakeFeedLinkAvailabilityPort struct {
	availability *domain.FeedLinkAvailability
	disabledNow  bool
	err          error
	gotURL       string
	gotReason    string
	gotThreshold int
}

func (f *fakeFeedLinkAvailabilityPort) RecordFailure(_ context.Context, feedURL, reason string, disableAfter int) (*domain.FeedLinkAvailability, bool, error) {
	f.gotURL, f.gotReason, f.gotThreshold = feedURL, reason, disableAfter
	return f.availability, f.disabledNow, f.err
}

func (f *fakeFeedLinkAvailabilityPort) ResetFailures(_ context.Context, feedURL string) error {
	f.gotURL = feedURL
	return f.err
}

type fakeFeedPort struct {
	rows        []*domain.FeedRow
	single      *domain.FeedRow
	random      *domain.Feed
	summary     *domain.FeedSummary
	results     []domain.FeedRegistrationResult
	titles      map[uuid.UUID]string
	err         error
	gotScope    datahub_capability_port.FeedScope
	gotUser     uuid.UUID
	gotCursor   *time.Time
	gotLimit    int
	gotExcludes []uuid.UUID
	gotUnread   bool
	gotUserPtr  *uuid.UUID
}

func (f *fakeFeedPort) Register(_ context.Context, _ []domain.FeedRegistration) ([]domain.FeedRegistrationResult, error) {
	return f.results, f.err
}

func (f *fakeFeedPort) ListCursor(_ context.Context, scope datahub_capability_port.FeedScope, userID uuid.UUID, cursor *time.Time, limit int, excludes []uuid.UUID) ([]*domain.FeedRow, error) {
	f.gotScope, f.gotUser, f.gotCursor, f.gotLimit, f.gotExcludes = scope, userID, cursor, limit, excludes
	return f.rows, f.err
}

func (f *fakeFeedPort) ListPage(_ context.Context, _ int, unreadOnly bool, userID uuid.UUID) ([]*domain.FeedRow, error) {
	f.gotUnread, f.gotUser = unreadOnly, userID
	return f.rows, f.err
}

func (f *fakeFeedPort) ListLimit(_ context.Context, limit int) ([]*domain.FeedRow, error) {
	f.gotLimit = limit
	return f.rows, f.err
}

func (f *fakeFeedPort) GetSingle(context.Context) (*domain.FeedRow, error) { return f.single, f.err }

func (f *fakeFeedPort) ListByFeedLinkID(context.Context, uuid.UUID) ([]*domain.FeedRow, error) {
	return f.rows, f.err
}

func (f *fakeFeedPort) GetSummary(_ context.Context, _ string, userID *uuid.UUID) (*domain.FeedSummary, error) {
	f.gotUserPtr = userID
	return f.summary, f.err
}

func (f *fakeFeedPort) GetSummaryByArticleID(_ context.Context, _ string, userID *uuid.UUID) (*domain.FeedSummary, error) {
	f.gotUserPtr = userID
	return f.summary, f.err
}

func (f *fakeFeedPort) SearchByTitle(context.Context, string, string) ([]*domain.FeedRow, error) {
	return f.rows, f.err
}

func (f *fakeFeedPort) GetRandom(context.Context) (*domain.Feed, error) { return f.random, f.err }

func (f *fakeFeedPort) GetFeedURLsByArticleIDs(context.Context, []string) ([]domain.FeedAndArticle, error) {
	return nil, f.err
}

func (f *fakeFeedPort) BatchGetTitlesByIDs(context.Context, []uuid.UUID) (map[uuid.UUID]string, error) {
	return f.titles, f.err
}

func (f *fakeFeedPort) GetInoreaderSummariesByURLs(context.Context, []string) ([]*domain.InoreaderSummary, error) {
	return nil, f.err
}

type batch3Fakes struct {
	feedLink     *fakeFeedLinkPort
	availability *fakeFeedLinkAvailabilityPort
	feed         *fakeFeedPort
}

func newBatch3Handler(f batch3Fakes) (*Handler, batch3Fakes) {
	if f.feedLink == nil {
		f.feedLink = &fakeFeedLinkPort{}
	}
	if f.availability == nil {
		f.availability = &fakeFeedLinkAvailabilityPort{}
	}
	if f.feed == nil {
		f.feed = &fakeFeedPort{}
	}

	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	WithWave3Batch3Capabilities(f.feedLink, f.availability, f.feed)(h)
	return h, f
}

const (
	batch3UserID     = "11111111-2222-3333-4444-555555555555"
	batch3FeedLinkID = "a1b2c3d4-1111-4111-8111-111111111111"
)

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

// A handler built without one of these would answer Unimplemented on the only
// route alt-backend has to the feed tables — which reads exactly like a
// retired procedure (CLAUDE.md rule 8, ADR-000928).
func TestWithWave3Batch3CapabilitiesRejectsNilCollaborators(t *testing.T) {
	feedLink := &fakeFeedLinkPort{}
	availability := &fakeFeedLinkAvailabilityPort{}
	feed := &fakeFeedPort{}

	tests := []struct {
		name         string
		feedLink     datahub_capability_port.FeedLinkPort
		availability datahub_capability_port.FeedLinkAvailabilityPort
		feed         datahub_capability_port.FeedPort
	}{
		{name: "nil feed link port", availability: availability, feed: feed},
		{name: "nil availability port", feedLink: feedLink, feed: feed},
		{name: "nil feed port", feedLink: feedLink, availability: availability},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				WithWave3Batch3Capabilities(tt.feedLink, tt.availability, tt.feed)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// §2.F Feed links
// ---------------------------------------------------------------------------

func TestRegisterFeedLinkReportsTheDuplicateRatherThanFailing(t *testing.T) {
	h, f := newBatch3Handler(batch3Fakes{feedLink: &fakeFeedLinkPort{alreadyExisted: true}})

	resp, err := h.RegisterFeedLink(context.Background(), connect.NewRequest(&datahubv1.RegisterFeedLinkRequest{
		Url: "https://example.com/feed.xml",
	}))

	require.NoError(t, err, "subscribing twice is a normal outcome, not an error")
	assert.True(t, resp.Msg.GetAlreadyExisted())
	assert.Equal(t, "https://example.com/feed.xml", f.feedLink.gotURL)
}

// An unsubscribed URL is an unset field, not an empty string. The registration
// flow reads the absence as "this is new"; an empty string would make every
// first-time registration look already-known.
func TestResolveFeedLinkIDByURLLeavesTheFieldUnsetOnAMiss(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{feedLink: &fakeFeedLinkPort{resolvedID: nil}})

	resp, err := h.ResolveFeedLinkIDByURL(context.Background(), connect.NewRequest(&datahubv1.ResolveFeedLinkIDByURLRequest{
		FeedUrl: "https://unknown.example.com/feed.xml",
	}))

	require.NoError(t, err)
	assert.Nil(t, resp.Msg.FeedLinkId)
}

// A never-polled link carries no availability message. The admin screen maps
// nil to "unknown" and a zero-failure row to "healthy", so a zero-valued
// struct here would report a feed nobody has ever checked as green.
func TestListFeedLinksWithHealthOmitsAvailabilityForNeverPolledLinks(t *testing.T) {
	linkID := uuid.MustParse(batch3FeedLinkID)
	h, _ := newBatch3Handler(batch3Fakes{feedLink: &fakeFeedLinkPort{
		withHealth: []*domain.FeedLinkWithHealth{{
			FeedLink: domain.FeedLink{ID: linkID, URL: "https://example.com/feed.xml"},
		}},
	}})

	resp, err := h.ListFeedLinksWithHealth(context.Background(), connect.NewRequest(&datahubv1.ListFeedLinksWithHealthRequest{}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.GetFeedLinks(), 1)
	assert.Nil(t, resp.Msg.GetFeedLinks()[0].GetAvailability())
}

func TestDeleteFeedLinkRefusesANonUUID(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.DeleteFeedLink(context.Background(), connect.NewRequest(&datahubv1.DeleteFeedLinkRequest{
		Id: "not-a-uuid",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ---------------------------------------------------------------------------
// §2.G Feed link availability (catalog §4-4)
// ---------------------------------------------------------------------------

// The threshold is the caller's and reaches the port unchanged. If the handler
// substituted a default the auto-disable policy would live in two places and
// only one of them would be configurable.
func TestRecordFeedLinkFailurePassesTheCallersThresholdThrough(t *testing.T) {
	linkID := uuid.MustParse(batch3FeedLinkID)
	h, f := newBatch3Handler(batch3Fakes{availability: &fakeFeedLinkAvailabilityPort{
		availability: &domain.FeedLinkAvailability{FeedLinkID: linkID, ConsecutiveFailures: 5},
		disabledNow:  true,
	}})

	resp, err := h.RecordFeedLinkFailure(context.Background(), connect.NewRequest(&datahubv1.RecordFeedLinkFailureRequest{
		FeedUrl:              "https://dead.example.com/feed.xml",
		Reason:               "404 Not Found",
		DisableAfterFailures: 5,
	}))

	require.NoError(t, err)
	assert.Equal(t, 5, f.availability.gotThreshold)
	assert.Equal(t, "404 Not Found", f.availability.gotReason)
	assert.True(t, resp.Msg.GetDisabledNow())
	assert.Equal(t, int32(5), resp.Msg.GetAvailability().GetConsecutiveFailures())
}

func TestRecordFeedLinkFailureRequiresAFeedURL(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.RecordFeedLinkFailure(context.Background(), connect.NewRequest(&datahubv1.RecordFeedLinkFailureRequest{
		Reason: "boom",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ---------------------------------------------------------------------------
// §2.H Feeds
// ---------------------------------------------------------------------------

func TestListFeedsCursorMapsEveryScope(t *testing.T) {
	tests := []struct {
		name  string
		wire  datahubv1.FeedScope
		want  datahub_capability_port.FeedScope
		exact bool
	}{
		{name: "all", wire: datahubv1.FeedScope_FEED_SCOPE_ALL, want: datahub_capability_port.FeedScopeAll},
		{name: "unread", wire: datahubv1.FeedScope_FEED_SCOPE_UNREAD, want: datahub_capability_port.FeedScopeUnread},
		{name: "read", wire: datahubv1.FeedScope_FEED_SCOPE_READ, want: datahub_capability_port.FeedScopeRead},
		{name: "favorite", wire: datahubv1.FeedScope_FEED_SCOPE_FAVORITE, want: datahub_capability_port.FeedScopeFavorite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := newBatch3Handler(batch3Fakes{})

			_, err := h.ListFeedsCursor(context.Background(), connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
				Scope:  tt.wire,
				UserId: batch3UserID,
				Limit:  20,
			}))

			require.NoError(t, err)
			assert.Equal(t, tt.want, f.feed.gotScope)
		})
	}
}

// An unset scope is refused rather than defaulting to ALL. The four scopes
// return different sets, so a caller that forgot the field would otherwise be
// handed every feed where it asked for unread ones — a wrong answer that looks
// like a right one.
func TestListFeedsCursorRefusesAnUnsetScope(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.ListFeedsCursor(context.Background(), connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
		UserId: batch3UserID,
		Limit:  20,
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// Every scope is one person's feeds, so a missing user_id is refused rather
// than treated as unscoped. The driver used to read the user from the request
// context, which cannot cross the wire; without this check the tenant
// predicate would simply be absent from the query.
func TestListFeedsCursorRequiresAUser(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.ListFeedsCursor(context.Background(), connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
		Scope: datahubv1.FeedScope_FEED_SCOPE_UNREAD,
		Limit: 20,
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListFeedsCursorRefusesAMalformedExclusion(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.ListFeedsCursor(context.Background(), connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
		Scope:              datahubv1.FeedScope_FEED_SCOPE_ALL,
		UserId:             batch3UserID,
		Limit:              20,
		ExcludeFeedLinkIds: []string{"not-a-uuid"},
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListFeedsCursorClampsTheLimit(t *testing.T) {
	h, f := newBatch3Handler(batch3Fakes{})

	_, err := h.ListFeedsCursor(context.Background(), connect.NewRequest(&datahubv1.ListFeedsCursorRequest{
		Scope:  datahubv1.FeedScope_FEED_SCOPE_ALL,
		UserId: batch3UserID,
		Limit:  100000,
	}))

	require.NoError(t, err)
	assert.Equal(t, maxFeedLimit, f.feed.gotLimit)
}

// user_id is required for the unread page and ignored for the unscoped one.
// Requiring it for both would break the plain list; accepting its absence for
// the unread page would answer with every user's feeds.
func TestListFeedsPageRequiresAUserOnlyForTheUnreadScope(t *testing.T) {
	t.Run("unread without a user is refused", func(t *testing.T) {
		h, _ := newBatch3Handler(batch3Fakes{})

		_, err := h.ListFeedsPage(context.Background(), connect.NewRequest(&datahubv1.ListFeedsPageRequest{
			Page:       0,
			UnreadOnly: true,
		}))

		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("unscoped without a user is served", func(t *testing.T) {
		h, f := newBatch3Handler(batch3Fakes{})

		_, err := h.ListFeedsPage(context.Background(), connect.NewRequest(&datahubv1.ListFeedsPageRequest{Page: 0}))

		require.NoError(t, err)
		assert.False(t, f.feed.gotUnread)
		assert.Equal(t, uuid.Nil, f.feed.gotUser)
	})
}

// An empty feeds table is an unset field, not NotFound: a fresh install has no
// feeds, and answering an error would make an empty database look broken.
func TestGetSingleFeedLeavesTheFieldUnsetOnAnEmptyTable(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{feed: &fakeFeedPort{single: nil}})

	resp, err := h.GetSingleFeed(context.Background(), connect.NewRequest(&datahubv1.GetSingleFeedRequest{}))

	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GetFeed())
}

func TestGetRandomFeedLeavesTheFieldUnsetWhenNothingIsTagged(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{feed: &fakeFeedPort{random: nil}})

	resp, err := h.GetRandomFeed(context.Background(), connect.NewRequest(&datahubv1.GetRandomFeedRequest{}))

	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GetFeed())
}

// An absent user_id selects the unscoped query rather than defaulting to some
// user — the fallback these reads have always had for service-to-service
// callers.
func TestGetArticleSummaryByArticleIDTreatsAnAbsentUserAsUnscoped(t *testing.T) {
	h, f := newBatch3Handler(batch3Fakes{})

	_, err := h.GetArticleSummaryByArticleID(context.Background(), connect.NewRequest(&datahubv1.GetArticleSummaryByArticleIDRequest{
		ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
	}))

	require.NoError(t, err)
	assert.Nil(t, f.feed.gotUserPtr)
}

// A malformed user_id is refused rather than silently treated as unscoped,
// which would widen a tenant-scoped read into a global one.
func TestGetArticleSummaryByArticleIDRefusesAMalformedUser(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.GetArticleSummaryByArticleID(context.Background(), connect.NewRequest(&datahubv1.GetArticleSummaryByArticleIDRequest{
		ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
		UserId:    strPtr("not-a-uuid"),
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// A zero pub_date stays unset. Many RSS items carry no publication date and
// the driver scans the zero value; encoding it as year 1 would sort every such
// feed to the bottom of a client that trusts the field.
func TestListFeedsLimitLeavesAZeroPubDateUnset(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{feed: &fakeFeedPort{rows: []*domain.FeedRow{{
		ID:         "b2c3d4e5-2222-4222-8222-222222222222",
		Title:      "No date",
		WebsiteURL: "https://example.com/post",
		CreatedAt:  time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
	}}}})

	resp, err := h.ListFeedsLimit(context.Background(), connect.NewRequest(&datahubv1.ListFeedsLimitRequest{Limit: 10}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.GetFeeds(), 1)
	assert.Nil(t, resp.Msg.GetFeeds()[0].GetPubDate())
	assert.NotNil(t, resp.Msg.GetFeeds()[0].GetCreatedAt())
}

// RegisterFeeds refuses a batch containing an entry with no website_url. The
// upsert keys on that column, so a blank one would collide every such item
// onto a single row rather than failing.
func TestRegisterFeedsRefusesAnEntryWithNoWebsiteURL(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{})

	_, err := h.RegisterFeeds(context.Background(), connect.NewRequest(&datahubv1.RegisterFeedsRequest{
		Feeds: []*datahubv1.FeedRegistration{{Title: "No URL"}},
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestFeedReadsSurfaceAsInternalRatherThanAsAnEmptyList(t *testing.T) {
	h, _ := newBatch3Handler(batch3Fakes{feed: &fakeFeedPort{err: errors.New("connection reset")}})

	_, err := h.ListFeedsLimit(context.Background(), connect.NewRequest(&datahubv1.ListFeedsLimitRequest{Limit: 10}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func strPtr(s string) *string { return &s }
