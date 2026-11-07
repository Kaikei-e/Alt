package repository

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	"pre-processor/driver"
	backend_api "pre-processor/driver/backend_api"
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

	feedIDByFeedURL      map[string]string // feed_links.url → feeds.id
	emptyFeedIDByFeedURL map[string]string // feed_links.url → empty feeds.id
	existing             map[string]string // article url → feeds.id it already lives under
	getEmptyFeedIDErr    error
	checkExistsErr       error

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

func (f *fakeDataHub) GetEmptyFeedID(_ context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
	if f.getEmptyFeedIDErr != nil {
		return nil, f.getEmptyFeedIDErr
	}
	feedID := f.emptyFeedIDByFeedURL[req.Msg.FeedUrl]
	return connect.NewResponse(&datahubv1.GetEmptyFeedIDResponse{FeedId: feedID}), nil
}

func (f *fakeDataHub) CheckArticleExists(_ context.Context, req *connect.Request[datahubv1.CheckArticleExistsRequest]) (*connect.Response[datahubv1.CheckArticleExistsResponse], error) {
	f.mu.Lock()
	f.existsChecks = append(f.existsChecks, req.Msg)
	f.mu.Unlock()

	if f.checkExistsErr != nil {
		return nil, f.checkExistsErr
	}

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

func newBackfillTestRepo(t *testing.T, fake *fakeDataHub) *articleRepository {
	t.Helper()

	path, handler := datahubv1connect.NewDataHubServiceHandler(fake)
	mux := http.NewServeMux()
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return NewArticleRepository(backend_api.NewClient(srv.URL, "", srv.Client()), nil)
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

func newStubBackfillPool(t *testing.T, rows []driver.InoreaderArticleRow) *pgxpool.Pool {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				backend := pgproto3.NewBackend(c, c)
				if _, err := backend.ReceiveStartupMessage(); err != nil {
					return
				}
				backend.Send(&pgproto3.AuthenticationOk{})
				backend.Send(&pgproto3.ParameterStatus{Name: "server_version", Value: "16.0"})
				backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
				backend.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
				backend.Send(&pgproto3.ParameterStatus{Name: "DateStyle", Value: "ISO, MDY"})
				backend.Send(&pgproto3.ParameterStatus{Name: "TimeZone", Value: "UTC"})
				backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
				if err := backend.Flush(); err != nil {
					return
				}

				for {
					msg, err := backend.Receive()
					if err != nil {
						return
					}
					switch msg.(type) {
					case *pgproto3.Query:
						backend.Send(&pgproto3.RowDescription{
							Fields: []pgproto3.FieldDescription{
								{Name: []byte("id"), DataTypeOID: pgtype.TextOID, Format: pgproto3.TextFormat},
								{Name: []byte("article_url"), DataTypeOID: pgtype.TextOID, Format: pgproto3.TextFormat},
								{Name: []byte("title"), DataTypeOID: pgtype.TextOID, Format: pgproto3.TextFormat},
								{Name: []byte("content"), DataTypeOID: pgtype.TextOID, Format: pgproto3.TextFormat},
								{Name: []byte("published_at"), DataTypeOID: pgtype.TimestamptzOID, Format: pgproto3.TextFormat},
								{Name: []byte("feed_url"), DataTypeOID: pgtype.TextOID, Format: pgproto3.TextFormat},
								{Name: []byte("fetched_at"), DataTypeOID: pgtype.TimestamptzOID, Format: pgproto3.TextFormat},
							},
						})
						for _, row := range rows {
							backend.Send(&pgproto3.DataRow{
								Values: [][]byte{
									[]byte(row.ID),
									[]byte(row.ArticleURL),
									[]byte(row.Title),
									[]byte(row.Content),
									[]byte(row.PublishedAt.UTC().Format("2006-01-02 15:04:05.999999Z07:00")),
									[]byte(row.FeedURL),
									[]byte(row.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999Z07:00")),
								},
							})
						}
						backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT")})
						backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
						if err := backend.Flush(); err != nil {
							return
						}
					case *pgproto3.Terminate:
						return
					}
				}
			}(conn)
		}
	}()

	dsn := "postgres://postgres@127.0.0.1:" + intToStr(ln.Addr().(*net.TCPAddr).Port) + "/postgres?sslmode=disable&default_query_exec_mode=simple_protocol"
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestFetchInoreaderArticlesForEmptyFeeds_GetEmptyFeedIDTransientErrorAbortsBatch(t *testing.T) {
	fetchedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	candidates := []driver.InoreaderArticleRow{{
		ID:          "ino-1",
		ArticleURL:  "https://example.com/articles/go-128",
		Title:       "Go 1.28",
		Content:     "<p>Body</p>",
		PublishedAt: fetchedAt.Add(-time.Hour),
		FeedURL:     "https://example.com/feed.xml",
		FetchedAt:   fetchedAt,
	}}

	t.Run("transient error returns error and does not advance scannedThrough", func(t *testing.T) {
		fake := &fakeDataHub{
			getEmptyFeedIDErr: connect.NewError(connect.CodeUnavailable, errors.New("datahub unavailable")),
		}
		path, handler := datahubv1connect.NewDataHubServiceHandler(fake)
		mux := http.NewServeMux()
		mux.Handle(path, handler)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		pool := newStubBackfillPool(t, candidates)
		repo := NewArticleRepository(backend_api.NewClient(srv.URL, "", srv.Client()), pool)

		articles, scannedThrough, err := repo.FetchInoreaderArticlesForEmptyFeeds(context.Background(), time.Time{}, 100)
		require.Error(t, err, "transient GetEmptyFeedID error must abort the batch instead of silently advancing the backfill cursor")
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		assert.Empty(t, articles)
		assert.True(t, scannedThrough.IsZero(), "scannedThrough must be zero when the batch errors out")
	})

	t.Run("NotFound error keeps skip semantics and advances scannedThrough", func(t *testing.T) {
		fake := &fakeDataHub{
			getEmptyFeedIDErr: connect.NewError(connect.CodeNotFound, errors.New("feed not found")),
		}
		path, handler := datahubv1connect.NewDataHubServiceHandler(fake)
		mux := http.NewServeMux()
		mux.Handle(path, handler)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		pool := newStubBackfillPool(t, candidates)
		repo := NewArticleRepository(backend_api.NewClient(srv.URL, "", srv.Client()), pool)

		articles, scannedThrough, err := repo.FetchInoreaderArticlesForEmptyFeeds(context.Background(), time.Time{}, 100)
		require.NoError(t, err)
		assert.Empty(t, articles)
		assert.Equal(t, fetchedAt, scannedThrough)
	})
}

func TestUpsertArticlesWithFeedID_CheckArticleExistsTransientErrorAbortsBatch(t *testing.T) {
	t.Run("transient error from CheckArticleExists is returned", func(t *testing.T) {
		fake := &fakeDataHub{
			checkExistsErr: connect.NewError(connect.CodeUnavailable, errors.New("datahub unavailable")),
		}
		repo := newBackfillTestRepo(t, fake)

		err := repo.UpsertArticlesWithFeedID(context.Background(), []*domain.Article{{
			URL:     "https://example.com/articles/go-128",
			Title:   "Go 1.28",
			Content: "RSS summary only.",
			FeedID:  "feed-001",
			UserID:  "user-001",
		}})
		require.Error(t, err, "transient CheckArticleExists error must abort the batch")
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))

		_, created, _ := fake.snapshot()
		assert.Empty(t, created)
	})

	t.Run("NotFound error from CheckArticleExists keeps skip semantics", func(t *testing.T) {
		fake := &fakeDataHub{
			checkExistsErr: connect.NewError(connect.CodeNotFound, errors.New("not found")),
		}
		repo := newBackfillTestRepo(t, fake)

		err := repo.UpsertArticlesWithFeedID(context.Background(), []*domain.Article{{
			URL:     "https://example.com/articles/go-128",
			Title:   "Go 1.28",
			Content: "RSS summary only.",
			FeedID:  "feed-001",
			UserID:  "user-001",
		}})
		require.NoError(t, err)

		_, created, _ := fake.snapshot()
		assert.Empty(t, created)
	})
}
