package datahubapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type fakeArticleWritePort struct {
	gotURL     string
	gotTitle   string
	gotContent string
	gotUser    uuid.UUID
	articleID  string
	created    bool
	err        error
}

func (f *fakeArticleWritePort) Archive(_ context.Context, url, title, content string, userID uuid.UUID) (string, bool, error) {
	f.gotURL, f.gotTitle, f.gotContent, f.gotUser = url, title, content, userID
	return f.articleID, f.created, f.err
}

type fakeArticleReadPort struct {
	content      *domain.ArticleContent
	byURL        map[string]*domain.ArticleContent
	articles     []*domain.Article
	ids          []uuid.UUID
	url          string
	title        string
	err          error
	gotUser      *uuid.UUID
	gotCursor    *time.Time
	gotLimit     int
	gotIDs       []uuid.UUID
	gotFeedID    uuid.UUID
	gotArticleID string
}

func (f *fakeArticleReadPort) GetByURL(_ context.Context, _ string, userID *uuid.UUID) (*domain.ArticleContent, error) {
	f.gotUser = userID
	return f.content, f.err
}

func (f *fakeArticleReadPort) BatchGetByURLs(_ context.Context, _ []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error) {
	f.gotUser = userID
	return f.byURL, f.err
}

func (f *fakeArticleReadPort) GetContentByID(_ context.Context, articleID string) (*domain.ArticleContent, error) {
	f.gotArticleID = articleID
	return f.content, f.err
}

func (f *fakeArticleReadPort) ListCursor(_ context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*domain.Article, error) {
	f.gotUser, f.gotCursor, f.gotLimit = &userID, cursor, limit
	return f.articles, f.err
}

func (f *fakeArticleReadPort) ListIDsCursor(_ context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]uuid.UUID, error) {
	f.gotUser, f.gotCursor, f.gotLimit = &userID, cursor, limit
	return f.ids, f.err
}

func (f *fakeArticleReadPort) BatchGetByIDs(_ context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error) {
	f.gotIDs = articleIDs
	return f.articles, f.err
}

func (f *fakeArticleReadPort) GetLatestByFeedID(_ context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	f.gotFeedID = feedID
	return f.content, f.err
}

func (f *fakeArticleReadPort) LookupSource(_ context.Context, articleID string, userID uuid.UUID) (domain.ArticleSource, error) {
	f.gotArticleID, f.gotUser = articleID, &userID
	return domain.ArticleSource{URL: f.url, Title: f.title}, f.err
}

type fakeKnowledgeBackfillPort struct {
	articleCount   int
	titleCount     int
	articles       []domain.KnowledgeBackfillArticle
	titles         []domain.KnowledgeBackfillSummaryTitle
	gotCreatedAt   *time.Time
	gotArticleID   *uuid.UUID
	gotGeneratedAt *time.Time
	gotVersionID   *uuid.UUID
	gotLimit       int
}

func (f *fakeKnowledgeBackfillPort) CountArticles(_ context.Context) (int, error) {
	return f.articleCount, nil
}

func (f *fakeKnowledgeBackfillPort) ListArticles(_ context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	f.gotCreatedAt, f.gotArticleID, f.gotLimit = lastCreatedAt, lastArticleID, limit
	return f.articles, nil
}

func (f *fakeKnowledgeBackfillPort) CountSummaryTitles(_ context.Context) (int, error) {
	return f.titleCount, nil
}

func (f *fakeKnowledgeBackfillPort) ListSummaryTitles(_ context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	f.gotGeneratedAt, f.gotVersionID, f.gotLimit = lastGeneratedAt, lastSummaryVersionID, limit
	return f.titles, nil
}

type batch2Fakes struct {
	write    *fakeArticleWritePort
	read     *fakeArticleReadPort
	backfill *fakeKnowledgeBackfillPort
}

func newBatch2Handler(f batch2Fakes) (*Handler, batch2Fakes) {
	if f.write == nil {
		f.write = &fakeArticleWritePort{}
	}
	if f.read == nil {
		f.read = &fakeArticleReadPort{}
	}
	if f.backfill == nil {
		f.backfill = &fakeKnowledgeBackfillPort{}
	}

	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	WithWave3Batch2Capabilities(f.write, f.read, f.backfill)(h)
	return h, f
}

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

// A handler built without one of these would answer Unimplemented on the only
// route alt-backend has to the articles table, which reads exactly like a
// retired procedure (CLAUDE.md rule 8, ADR-000928).
func TestWithWave3Batch2CapabilitiesRejectsNilCollaborators(t *testing.T) {
	read := &fakeArticleReadPort{}
	write := &fakeArticleWritePort{}
	backfill := &fakeKnowledgeBackfillPort{}

	tests := []struct {
		name string
		call func()
	}{
		{"nil article write", func() { WithWave3Batch2Capabilities(nil, read, backfill) }},
		{"nil article read", func() { WithWave3Batch2Capabilities(write, nil, backfill) }},
		{"nil knowledge backfill", func() { WithWave3Batch2Capabilities(write, read, nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, tt.call)
		})
	}
}

// ---------------------------------------------------------------------------
// §2.B Article writes
// ---------------------------------------------------------------------------

func TestArchiveArticle_PassesOwnerThrough(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	h, f := newBatch2Handler(batch2Fakes{write: &fakeArticleWritePort{
		articleID: "22222222-2222-4222-8222-222222222222",
		created:   true,
	}})

	resp, err := h.ArchiveArticle(context.Background(), connect.NewRequest(&datahubv1.ArchiveArticleRequest{
		Url:     "https://example.com/a",
		Title:   "Title",
		Content: "body",
		UserId:  userID.String(),
	}))
	require.NoError(t, err)
	assert.Equal(t, "22222222-2222-4222-8222-222222222222", resp.Msg.GetArticleId())
	assert.True(t, resp.Msg.GetCreated())
	assert.Equal(t, userID, f.write.gotUser, "the owner must survive the wire, not be re-derived")
	assert.Equal(t, "https://example.com/a", f.write.gotURL)
}

func TestArchiveArticle_RejectsBadRequests(t *testing.T) {
	tests := []struct {
		name string
		req  *datahubv1.ArchiveArticleRequest
	}{
		{"missing url", &datahubv1.ArchiveArticleRequest{UserId: uuid.NewString(), Content: "body"}},
		{"missing user", &datahubv1.ArchiveArticleRequest{Url: "https://example.com/a", Content: "body"}},
		{"user is not a uuid", &datahubv1.ArchiveArticleRequest{Url: "https://example.com/a", UserId: "nobody", Content: "body"}},
		{"missing content", &datahubv1.ArchiveArticleRequest{Url: "https://example.com/a", UserId: uuid.NewString()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newBatch2Handler(batch2Fakes{})
			_, err := h.ArchiveArticle(context.Background(), connect.NewRequest(tt.req))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestArchiveArticle_PropagatesFailureAsInternal(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{write: &fakeArticleWritePort{err: errors.New("tx aborted")}})

	_, err := h.ArchiveArticle(context.Background(), connect.NewRequest(&datahubv1.ArchiveArticleRequest{
		Url:     "https://example.com/a",
		Content: "body",
		UserId:  uuid.NewString(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "tx aborted", "the driver's message must not cross the boundary")
}

func TestSaveArticleHead_Succeeds(t *testing.T) {
	og := &fakeOgImagePort{}
	h, _ := newWave3Handler(wave3Fakes{ogImage: og})

	_, err := h.SaveArticleHead(context.Background(), connect.NewRequest(&datahubv1.SaveArticleHeadRequest{
		ArticleId:  "22222222-2222-4222-8222-222222222222",
		HeadHtml:   "<head></head>",
		OgImageUrl: "https://cdn.example.com/og.png",
	}))
	require.NoError(t, err)
	assert.Equal(t, "22222222-2222-4222-8222-222222222222", og.savedHeadArticleID)
	assert.Equal(t, "<head></head>", og.savedHeadHTML)
}

func TestSaveArticleHead_RequiresArticleID(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	_, err := h.SaveArticleHead(context.Background(), connect.NewRequest(&datahubv1.SaveArticleHeadRequest{
		HeadHtml: "<head></head>",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ---------------------------------------------------------------------------
// §2.C Article reads
// ---------------------------------------------------------------------------

func TestGetArticleByURL_ScopeIsChosenByTheCaller(t *testing.T) {
	userID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	tests := []struct {
		name     string
		userID   string
		wantUser *uuid.UUID
	}{
		{"scoped when user_id is set", userID.String(), &userID},
		{"unscoped when user_id is empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
				content: &domain.ArticleContent{ID: "a1", Title: "T", Content: "C", URL: "https://example.com/a", FeedID: "f1"},
			}})

			resp, err := h.GetArticleByURL(context.Background(), connect.NewRequest(&datahubv1.GetArticleByURLRequest{
				Url:    "https://example.com/a",
				UserId: tt.userID,
			}))
			require.NoError(t, err)
			assert.Equal(t, "a1", resp.Msg.GetArticle().GetId())
			assert.Equal(t, tt.wantUser, f.read.gotUser)
		})
	}
}

// An unarchived URL leaves the field unset. The caller reads that absence as
// "go fetch the page"; an empty ArticleContent would look like an article with
// no body.
func TestGetArticleByURL_MissLeavesTheFieldUnset(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{content: nil}})

	resp, err := h.GetArticleByURL(context.Background(), connect.NewRequest(&datahubv1.GetArticleByURLRequest{
		Url: "https://example.com/missing",
	}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GetArticle())
}

func TestGetArticleByURL_RequiresURL(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.GetArticleByURL(context.Background(), connect.NewRequest(&datahubv1.GetArticleByURLRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestBatchGetArticlesByURLs_EmptyRequestSkipsTheQuery(t *testing.T) {
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		byURL: map[string]*domain.ArticleContent{"x": {ID: "should not be reached"}},
	}})

	resp, err := h.BatchGetArticlesByURLs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByURLsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetArticles())
	assert.Nil(t, f.read.gotUser, "no port call means the scope was never resolved")
}

func TestBatchGetArticlesByURLs_RejectsOversizedBatch(t *testing.T) {
	urls := make([]string, maxLimit+1)
	for i := range urls {
		urls[i] = "https://example.com/a"
	}

	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.BatchGetArticlesByURLs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByURLsRequest{Urls: urls}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestBatchGetArticlesByURLs_OmitsMisses(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		byURL: map[string]*domain.ArticleContent{
			"https://example.com/a": {ID: "a1", Title: "T", URL: "https://example.com/a"},
		},
	}})

	resp, err := h.BatchGetArticlesByURLs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByURLsRequest{
		Urls: []string{"https://example.com/a", "https://example.com/b"},
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetArticles(), 1)
	assert.Equal(t, "a1", resp.Msg.GetArticles()["https://example.com/a"].GetId())
}

func TestGetArticleContentByID(t *testing.T) {
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		content: &domain.ArticleContent{ID: "a1", Title: "T", Content: "C", URL: "https://example.com/a"},
	}})

	resp, err := h.GetArticleContentByID(context.Background(), connect.NewRequest(&datahubv1.GetArticleContentByIDRequest{
		ArticleId: "a1",
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a", resp.Msg.GetArticle().GetUrl())
	assert.Equal(t, "a1", f.read.gotArticleID)
}

func TestGetArticleContentByID_RequiresArticleID(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.GetArticleContentByID(context.Background(), connect.NewRequest(&datahubv1.GetArticleContentByIDRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListArticlesCursor_PassesCursorAndClampsLimit(t *testing.T) {
	userID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	cursor := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	feedID := uuid.New()
	articleID := uuid.New()

	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		articles: []*domain.Article{{
			ID: articleID, FeedID: feedID, Title: "T", Content: "C", URL: "https://example.com/a",
			Tags: []string{"go"}, PublishedAt: cursor, CreatedAt: cursor,
		}},
	}})

	resp, err := h.ListArticlesCursor(context.Background(), connect.NewRequest(&datahubv1.ListArticlesCursorRequest{
		UserId: userID.String(),
		Cursor: timestamppb.New(cursor),
		Limit:  100000,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetArticles(), 1)
	assert.Equal(t, articleID.String(), resp.Msg.GetArticles()[0].GetId())
	assert.Equal(t, []string{"go"}, resp.Msg.GetArticles()[0].GetTags())
	require.NotNil(t, f.read.gotCursor)
	assert.True(t, cursor.Equal(*f.read.gotCursor))
	assert.Equal(t, maxLimit, f.read.gotLimit, "an unbounded page size would let one call read the whole table")
}

func TestListArticlesCursor_FirstPageHasNoCursor(t *testing.T) {
	h, f := newBatch2Handler(batch2Fakes{})

	_, err := h.ListArticlesCursor(context.Background(), connect.NewRequest(&datahubv1.ListArticlesCursorRequest{
		UserId: uuid.NewString(),
	}))
	require.NoError(t, err)
	assert.Nil(t, f.read.gotCursor, "an unset cursor must stay nil, not become the zero time")
}

func TestListArticlesCursor_RequiresUser(t *testing.T) {
	tests := []struct{ name, userID string }{
		{"missing", ""},
		{"not a uuid", "nobody"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newBatch2Handler(batch2Fakes{})
			_, err := h.ListArticlesCursor(context.Background(), connect.NewRequest(&datahubv1.ListArticlesCursorRequest{
				UserId: tt.userID,
			}))
			require.Error(t, err, "a per-user timeline with no user would page through everybody's articles")
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestListArticleIDsCursor(t *testing.T) {
	id := uuid.New()
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{ids: []uuid.UUID{id}}})

	resp, err := h.ListArticleIDsCursor(context.Background(), connect.NewRequest(&datahubv1.ListArticleIDsCursorRequest{
		UserId: uuid.NewString(),
		Limit:  50,
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{id.String()}, resp.Msg.GetArticleIds())
	assert.Equal(t, 50, f.read.gotLimit)
}

func TestBatchGetArticlesByIDs_PreservesRequestedOrder(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		articles: []*domain.Article{{ID: first, Title: "first"}, {ID: second, Title: "second"}},
	}})

	resp, err := h.BatchGetArticlesByIDs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByIDsRequest{
		ArticleIds: []string{first.String(), second.String()},
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetArticles(), 2)
	assert.Equal(t, "first", resp.Msg.GetArticles()[0].GetTitle())
	assert.Equal(t, []uuid.UUID{first, second}, f.read.gotIDs)
}

func TestBatchGetArticlesByIDs_RejectsUnparseableID(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.BatchGetArticlesByIDs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByIDsRequest{
		ArticleIds: []string{"not-a-uuid"},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestBatchGetArticlesByIDs_EmptyRequestSkipsTheQuery(t *testing.T) {
	h, f := newBatch2Handler(batch2Fakes{})
	resp, err := h.BatchGetArticlesByIDs(context.Background(), connect.NewRequest(&datahubv1.BatchGetArticlesByIDsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetArticles())
	assert.Nil(t, f.read.gotIDs)
}

func TestGetLatestArticleByFeedID_MissLeavesTheFieldUnset(t *testing.T) {
	feedID := uuid.New()
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{content: nil}})

	resp, err := h.GetLatestArticleByFeedID(context.Background(), connect.NewRequest(&datahubv1.GetLatestArticleByFeedIDRequest{
		FeedId: feedID.String(),
	}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GetArticle(), "a feed registered but not yet crawled is not an error")
	assert.Equal(t, feedID, f.read.gotFeedID)
}

func TestGetLatestArticleByFeedID_RejectsUnparseableFeedID(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.GetLatestArticleByFeedID(context.Background(), connect.NewRequest(&datahubv1.GetLatestArticleByFeedIDRequest{
		FeedId: "nope",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestLookupArticleURL(t *testing.T) {
	userID := uuid.New()
	h, f := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{
		url:   "https://example.com/a",
		title: "Working at a toy store",
	}})

	resp, err := h.LookupArticleURL(context.Background(), connect.NewRequest(&datahubv1.LookupArticleURLRequest{
		ArticleId: "a1",
		UserId:    userID.String(),
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/a", resp.Msg.GetUrl())
	// The title travels with the URL: the caller resolving an id has no other
	// way to learn what the article is called, and derives a headline from the
	// URL host when it is absent.
	assert.Equal(t, "Working at a toy store", resp.Msg.GetTitle())
	assert.Equal(t, "a1", f.read.gotArticleID)
	require.NotNil(t, f.read.gotUser)
	assert.Equal(t, userID, *f.read.gotUser)
}

// A row with no stored title is still a hit — the URL decides found from
// not-found, and the caller renders around the gap.
func TestLookupArticleURL_UntitledArticleKeepsItsURL(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{url: "https://example.com/untitled"}})

	resp, err := h.LookupArticleURL(context.Background(), connect.NewRequest(&datahubv1.LookupArticleURLRequest{
		ArticleId: "a1",
		UserId:    uuid.NewString(),
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/untitled", resp.Msg.GetUrl())
	assert.Empty(t, resp.Msg.GetTitle())
}

// An article outside the tenant answers "" and not NotFound: a distinct code
// would confirm that somebody else's article exists.
func TestLookupArticleURL_ForeignArticleIsEmptyNotNotFound(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{read: &fakeArticleReadPort{url: ""}})

	resp, err := h.LookupArticleURL(context.Background(), connect.NewRequest(&datahubv1.LookupArticleURLRequest{
		ArticleId: "someone-elses",
		UserId:    uuid.NewString(),
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetUrl())
}

func TestLookupArticleURL_RequiresBothIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		req  *datahubv1.LookupArticleURLRequest
	}{
		{"missing article", &datahubv1.LookupArticleURLRequest{UserId: uuid.NewString()}},
		{"missing user", &datahubv1.LookupArticleURLRequest{ArticleId: "a1"}},
		{"user is not a uuid", &datahubv1.LookupArticleURLRequest{ArticleId: "a1", UserId: "nobody"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newBatch2Handler(batch2Fakes{})
			_, err := h.LookupArticleURL(context.Background(), connect.NewRequest(tt.req))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

// ---------------------------------------------------------------------------
// §2.N Knowledge backfill
// ---------------------------------------------------------------------------

func TestCountBackfillArticles(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{backfill: &fakeKnowledgeBackfillPort{articleCount: 4242}})

	resp, err := h.CountBackfillArticles(context.Background(), connect.NewRequest(&datahubv1.CountBackfillArticlesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(4242), resp.Msg.GetCount())
}

func TestListBackfillArticles_FirstPageAndKeyset(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	articleID := uuid.New()
	userID := uuid.New()

	h, f := newBatch2Handler(batch2Fakes{backfill: &fakeKnowledgeBackfillPort{
		articles: []domain.KnowledgeBackfillArticle{{
			ArticleID: articleID, UserID: userID, CreatedAt: createdAt,
			PublishedAt: createdAt, Title: "T", URL: "https://example.com/a",
		}},
	}})

	resp, err := h.ListBackfillArticles(context.Background(), connect.NewRequest(&datahubv1.ListBackfillArticlesRequest{Limit: 10}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetArticles(), 1)
	assert.Equal(t, articleID.String(), resp.Msg.GetArticles()[0].GetArticleId())
	assert.Nil(t, f.backfill.gotCreatedAt, "first page sends no cursor")
	assert.Nil(t, f.backfill.gotArticleID)

	_, err = h.ListBackfillArticles(context.Background(), connect.NewRequest(&datahubv1.ListBackfillArticlesRequest{
		LastCreatedAt: timestamppb.New(createdAt),
		LastArticleId: articleID.String(),
		Limit:         10,
	}))
	require.NoError(t, err)
	require.NotNil(t, f.backfill.gotCreatedAt)
	require.NotNil(t, f.backfill.gotArticleID)
	assert.Equal(t, articleID, *f.backfill.gotArticleID)
}

// A half-set cursor is refused rather than silently restarting the walk. The
// job would replay from the beginning and re-emit every knowledge event it had
// already emitted.
func TestListBackfillArticles_RejectsHalfSetCursor(t *testing.T) {
	tests := []struct {
		name string
		req  *datahubv1.ListBackfillArticlesRequest
	}{
		{"timestamp without id", &datahubv1.ListBackfillArticlesRequest{LastCreatedAt: timestamppb.Now()}},
		{"id without timestamp", &datahubv1.ListBackfillArticlesRequest{LastArticleId: uuid.NewString()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newBatch2Handler(batch2Fakes{})
			_, err := h.ListBackfillArticles(context.Background(), connect.NewRequest(tt.req))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestCountBackfillSummaryTitles(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{backfill: &fakeKnowledgeBackfillPort{titleCount: 7}})

	resp, err := h.CountBackfillSummaryTitles(context.Background(), connect.NewRequest(&datahubv1.CountBackfillSummaryTitlesRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(7), resp.Msg.GetCount())
}

func TestListBackfillSummaryTitles(t *testing.T) {
	generatedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	versionID, articleID, userID := uuid.New(), uuid.New(), uuid.New()

	h, f := newBatch2Handler(batch2Fakes{backfill: &fakeKnowledgeBackfillPort{
		titles: []domain.KnowledgeBackfillSummaryTitle{{
			SummaryVersionID: versionID, ArticleID: articleID, UserID: userID,
			TenantID: userID, Title: "T", GeneratedAt: generatedAt,
		}},
	}})

	resp, err := h.ListBackfillSummaryTitles(context.Background(), connect.NewRequest(&datahubv1.ListBackfillSummaryTitlesRequest{
		LastGeneratedAt:      timestamppb.New(generatedAt),
		LastSummaryVersionId: versionID.String(),
		Limit:                5,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetEntries(), 1)
	assert.Equal(t, versionID.String(), resp.Msg.GetEntries()[0].GetSummaryVersionId())
	assert.Equal(t, 5, f.backfill.gotLimit)
	require.NotNil(t, f.backfill.gotVersionID)
}

func TestListBackfillSummaryTitles_RejectsHalfSetCursor(t *testing.T) {
	h, _ := newBatch2Handler(batch2Fakes{})
	_, err := h.ListBackfillSummaryTitles(context.Background(), connect.NewRequest(&datahubv1.ListBackfillSummaryTitlesRequest{
		LastSummaryVersionId: uuid.NewString(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
