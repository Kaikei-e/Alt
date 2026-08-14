package backend_api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
	"pre-processor/gen/proto/services/datahub/v1/datahubv1connect"
)

// fakeDataHub serves the slice of DataHubService the backfill path touches,
// reproducing the one behaviour a client-interface mock cannot show: GetFeedID
// resolves *feed* URLs only — alt-data-hub answers it by joining
// feed_links.url (shared/driver/alt_db/feed_url_to_id_driver.go), so handing it
// an article URL always comes back NotFound.
type fakeDataHub struct {
	datahubv1connect.UnimplementedDataHubServiceHandler

	feedIDByFeedURL map[string]string // feed_links.url → feeds.id
	existing        map[string]string // article url → feeds.id it already lives under

	mu            sync.Mutex
	getFeedIDURLs []string
	existsChecks  []*datahubv1.CheckArticleExistsRequest
	createdURLs   []string
}

func (f *fakeDataHub) GetFeedID(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
	f.mu.Lock()
	f.getFeedIDURLs = append(f.getFeedIDURLs, req.Msg.FeedUrl)
	f.mu.Unlock()

	feedID, ok := f.feedIDByFeedURL[req.Msg.FeedUrl]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found by URL"))
	}
	return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: feedID}), nil
}

func (f *fakeDataHub) CheckArticleExists(_ context.Context, req *connect.Request[datahubv1.CheckArticleExistsRequest]) (*connect.Response[datahubv1.CheckArticleExistsResponse], error) {
	f.mu.Lock()
	f.existsChecks = append(f.existsChecks, req.Msg)
	f.mu.Unlock()

	// Mirrors the handler's argument validation so a caller that forgets the
	// feed_id fails here the way it would in production.
	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	feedID, ok := f.existing[req.Msg.Url]
	return connect.NewResponse(&datahubv1.CheckArticleExistsResponse{
		Exists: ok && feedID == req.Msg.FeedId,
	}), nil
}

func (f *fakeDataHub) CreateArticle(_ context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
	f.mu.Lock()
	f.createdURLs = append(f.createdURLs, req.Msg.Url)
	f.mu.Unlock()

	return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "art-new"}), nil
}

func (f *fakeDataHub) snapshot() (feedLookups, created []string, checks []*datahubv1.CheckArticleExistsRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getFeedIDURLs, f.createdURLs, f.existsChecks
}

func newBackfillTestRepo(t *testing.T, fake *fakeDataHub) *ArticleRepository {
	t.Helper()

	path, handler := datahubv1connect.NewDataHubServiceHandler(fake)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return NewArticleRepository(NewClient(srv.URL, "", srv.Client()), nil)
}

// TestUpsertArticlesWithFeedID_SkipsArticleAlreadyStoredUnderItsFeed pins the
// DO NOTHING semantics the method promises: a backfill article whose URL is
// already stored under the resolved feed must not be sent to CreateArticle
// again, because the stored row may be a full-text fetch and the backfill row
// is only an Inoreader RSS summary.
//
// The existence check has to run against the FeedID the caller already
// resolved. Looking the feed up from the *article* URL cannot work — GetFeedID
// matches feed_links.url — so that path returns NotFound for every backfill
// article and the dedup silently degrades to "nothing exists".
func TestUpsertArticlesWithFeedID_SkipsArticleAlreadyStoredUnderItsFeed(t *testing.T) {
	fake := &fakeDataHub{
		feedIDByFeedURL: map[string]string{"https://example.com/feed.xml": "feed-001"},
		existing:        map[string]string{"https://example.com/articles/go-126": "feed-001"},
	}
	repo := newBackfillTestRepo(t, fake)

	err := repo.UpsertArticlesWithFeedID(context.Background(), []*domain.Article{{
		URL:     "https://example.com/articles/go-126",
		Title:   "Go 1.26 Released",
		Content: "RSS summary only.",
		FeedID:  "feed-001",
		UserID:  "user-001",
	}})
	require.NoError(t, err)

	feedLookups, created, checks := fake.snapshot()

	assert.Empty(t, created,
		"an article already stored under feed-001 must be skipped, not re-created over the full-text row")
	assert.Empty(t, feedLookups,
		"the caller already resolved FeedID; GetFeedID must not be re-issued on the article URL, which can never match feed_links.url")
	require.Len(t, checks, 1, "expected exactly one CheckArticleExists call")
	assert.Equal(t, "feed-001", checks[0].FeedId,
		"CheckArticleExists must carry the resolved FeedID")
	assert.Equal(t, "https://example.com/articles/go-126", checks[0].Url)
}

// TestUpsertArticlesWithFeedID_CreatesArticleAbsentFromItsFeed is the other
// half: the dedup must not turn into "skip everything". An article that is not
// yet stored under its feed still gets created.
func TestUpsertArticlesWithFeedID_CreatesArticleAbsentFromItsFeed(t *testing.T) {
	fake := &fakeDataHub{
		feedIDByFeedURL: map[string]string{"https://example.com/feed.xml": "feed-001"},
		existing:        map[string]string{},
	}
	repo := newBackfillTestRepo(t, fake)

	err := repo.UpsertArticlesWithFeedID(context.Background(), []*domain.Article{{
		URL:     "https://example.com/articles/go-127",
		Title:   "Go 1.27 Preview",
		Content: "RSS summary only.",
		FeedID:  "feed-001",
		UserID:  "user-001",
	}})
	require.NoError(t, err)

	feedLookups, created, _ := fake.snapshot()

	assert.Equal(t, []string{"https://example.com/articles/go-127"}, created)
	assert.Empty(t, feedLookups, "no GetFeedID round-trip is needed when FeedID is already resolved")
}
