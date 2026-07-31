package datahubapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	backendv1 "alt/gen/proto/services/backend/v1"
	"alt/gen/proto/services/backend/v1/backendv1connect"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestHandler builds a Handler with every dependency satisfied, because
// NewHandler refuses to build otherwise. Tests that care about one dependency
// override it; the rest get an inert stub so an unrelated test never depends
// on a collaborator it does not exercise.
func newTestHandler(legacy *spyLegacy, overrides ...func(*testDeps)) *Handler {
	deps := &testDeps{
		systemUser:     &fakeSystemUser{},
		recentArticles: &fakeRecentArticles{out: &fetch_recent_articles_usecase.FetchRecentArticlesOutput{}},
	}
	for _, o := range overrides {
		o(deps)
	}
	return NewHandler(legacy, deps.systemUser, deps.recentArticles, testLogger())
}

type testDeps struct {
	systemUser     SystemUserPort
	recentArticles RecentArticlesUsecase
}

func withSystemUser(p SystemUserPort) func(*testDeps) {
	return func(d *testDeps) { d.systemUser = p }
}

func withRecentArticles(u RecentArticlesUsecase) func(*testDeps) {
	return func(d *testDeps) { d.recentArticles = u }
}

// -----------------------------------------------------------------------------
// spyLegacy — a BackendInternalServiceHandler that records which procedure was
// called and with what, and answers with a canned response.
// -----------------------------------------------------------------------------

type spyLegacy struct {
	calls  []string
	lastIn proto.Message
	canned proto.Message
	err    error
}

func (s *spyLegacy) record(name string, in proto.Message) {
	s.calls = append(s.calls, name)
	s.lastIn = in
}

// answer fills out with whatever canned response the test set, transcoded
// through the wire so the test double does not depend on the adapter's own
// conversion helper.
func (s *spyLegacy) answer(out proto.Message) error {
	if s.canned == nil {
		return nil
	}
	raw, err := proto.Marshal(s.canned)
	if err != nil {
		return err
	}
	return proto.Unmarshal(raw, out)
}

func legacyStub[Resp any, PResp protoPtr[Resp], Req any, PReq protoPtr[Req]](
	s *spyLegacy, name string, req *connect.Request[Req],
) (*connect.Response[Resp], error) {
	s.record(name, PReq(req.Msg))
	if s.err != nil {
		return nil, s.err
	}
	out := PResp(new(Resp))
	if err := s.answer(out); err != nil {
		return nil, err
	}
	return connect.NewResponse((*Resp)(out)), nil
}

func (s *spyLegacy) ListArticlesWithTags(_ context.Context, r *connect.Request[backendv1.ListArticlesWithTagsRequest]) (*connect.Response[backendv1.ListArticlesWithTagsResponse], error) {
	return legacyStub[backendv1.ListArticlesWithTagsResponse](s, "ListArticlesWithTags", r)
}

func (s *spyLegacy) ListArticlesWithTagsForward(_ context.Context, r *connect.Request[backendv1.ListArticlesWithTagsForwardRequest]) (*connect.Response[backendv1.ListArticlesWithTagsForwardResponse], error) {
	return legacyStub[backendv1.ListArticlesWithTagsForwardResponse](s, "ListArticlesWithTagsForward", r)
}

func (s *spyLegacy) ListDeletedArticles(_ context.Context, r *connect.Request[backendv1.ListDeletedArticlesRequest]) (*connect.Response[backendv1.ListDeletedArticlesResponse], error) {
	return legacyStub[backendv1.ListDeletedArticlesResponse](s, "ListDeletedArticles", r)
}

func (s *spyLegacy) GetLatestArticleTimestamp(_ context.Context, r *connect.Request[backendv1.GetLatestArticleTimestampRequest]) (*connect.Response[backendv1.GetLatestArticleTimestampResponse], error) {
	return legacyStub[backendv1.GetLatestArticleTimestampResponse](s, "GetLatestArticleTimestamp", r)
}

func (s *spyLegacy) GetArticleByID(_ context.Context, r *connect.Request[backendv1.GetArticleByIDRequest]) (*connect.Response[backendv1.GetArticleByIDResponse], error) {
	return legacyStub[backendv1.GetArticleByIDResponse](s, "GetArticleByID", r)
}

func (s *spyLegacy) CheckArticleExists(_ context.Context, r *connect.Request[backendv1.CheckArticleExistsRequest]) (*connect.Response[backendv1.CheckArticleExistsResponse], error) {
	return legacyStub[backendv1.CheckArticleExistsResponse](s, "CheckArticleExists", r)
}

func (s *spyLegacy) CreateArticle(_ context.Context, r *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
	return legacyStub[backendv1.CreateArticleResponse](s, "CreateArticle", r)
}

func (s *spyLegacy) SaveArticleSummary(_ context.Context, r *connect.Request[backendv1.SaveArticleSummaryRequest]) (*connect.Response[backendv1.SaveArticleSummaryResponse], error) {
	return legacyStub[backendv1.SaveArticleSummaryResponse](s, "SaveArticleSummary", r)
}

func (s *spyLegacy) GetArticleContent(_ context.Context, r *connect.Request[backendv1.GetArticleContentRequest]) (*connect.Response[backendv1.GetArticleContentResponse], error) {
	return legacyStub[backendv1.GetArticleContentResponse](s, "GetArticleContent", r)
}

func (s *spyLegacy) GetFeedID(_ context.Context, r *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
	return legacyStub[backendv1.GetFeedIDResponse](s, "GetFeedID", r)
}

func (s *spyLegacy) ListFeedURLs(_ context.Context, r *connect.Request[backendv1.ListFeedURLsRequest]) (*connect.Response[backendv1.ListFeedURLsResponse], error) {
	return legacyStub[backendv1.ListFeedURLsResponse](s, "ListFeedURLs", r)
}

func (s *spyLegacy) UpsertArticleTags(_ context.Context, r *connect.Request[backendv1.UpsertArticleTagsRequest]) (*connect.Response[backendv1.UpsertArticleTagsResponse], error) {
	return legacyStub[backendv1.UpsertArticleTagsResponse](s, "UpsertArticleTags", r)
}

func (s *spyLegacy) BatchUpsertArticleTags(_ context.Context, r *connect.Request[backendv1.BatchUpsertArticleTagsRequest]) (*connect.Response[backendv1.BatchUpsertArticleTagsResponse], error) {
	return legacyStub[backendv1.BatchUpsertArticleTagsResponse](s, "BatchUpsertArticleTags", r)
}

func (s *spyLegacy) ListUntaggedArticles(_ context.Context, r *connect.Request[backendv1.ListUntaggedArticlesRequest]) (*connect.Response[backendv1.ListUntaggedArticlesResponse], error) {
	return legacyStub[backendv1.ListUntaggedArticlesResponse](s, "ListUntaggedArticles", r)
}

func (s *spyLegacy) BatchGetTagsByArticleIDs(_ context.Context, r *connect.Request[backendv1.BatchGetTagsByArticleIDsRequest]) (*connect.Response[backendv1.BatchGetTagsByArticleIDsResponse], error) {
	return legacyStub[backendv1.BatchGetTagsByArticleIDsResponse](s, "BatchGetTagsByArticleIDs", r)
}

func (s *spyLegacy) DeleteArticleSummary(_ context.Context, r *connect.Request[backendv1.DeleteArticleSummaryRequest]) (*connect.Response[backendv1.DeleteArticleSummaryResponse], error) {
	return legacyStub[backendv1.DeleteArticleSummaryResponse](s, "DeleteArticleSummary", r)
}

func (s *spyLegacy) CheckArticleSummaryExists(_ context.Context, r *connect.Request[backendv1.CheckArticleSummaryExistsRequest]) (*connect.Response[backendv1.CheckArticleSummaryExistsResponse], error) {
	return legacyStub[backendv1.CheckArticleSummaryExistsResponse](s, "CheckArticleSummaryExists", r)
}

func (s *spyLegacy) FindArticlesWithSummaries(_ context.Context, r *connect.Request[backendv1.FindArticlesWithSummariesRequest]) (*connect.Response[backendv1.FindArticlesWithSummariesResponse], error) {
	return legacyStub[backendv1.FindArticlesWithSummariesResponse](s, "FindArticlesWithSummaries", r)
}

func (s *spyLegacy) ListUnsummarizedArticles(_ context.Context, r *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
	return legacyStub[backendv1.ListUnsummarizedArticlesResponse](s, "ListUnsummarizedArticles", r)
}

func (s *spyLegacy) HasUnsummarizedArticles(_ context.Context, r *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
	return legacyStub[backendv1.HasUnsummarizedArticlesResponse](s, "HasUnsummarizedArticles", r)
}

func (s *spyLegacy) GetEmptyFeedID(_ context.Context, r *connect.Request[backendv1.GetEmptyFeedIDRequest]) (*connect.Response[backendv1.GetEmptyFeedIDResponse], error) {
	return legacyStub[backendv1.GetEmptyFeedIDResponse](s, "GetEmptyFeedID", r)
}

func (s *spyLegacy) FetchTagCloud(_ context.Context, r *connect.Request[backendv1.BackendInternalServiceFetchTagCloudRequest]) (*connect.Response[backendv1.BackendInternalServiceFetchTagCloudResponse], error) {
	return legacyStub[backendv1.BackendInternalServiceFetchTagCloudResponse](s, "FetchTagCloud", r)
}

func (s *spyLegacy) FetchArticlesByTag(_ context.Context, r *connect.Request[backendv1.BackendInternalServiceFetchArticlesByTagRequest]) (*connect.Response[backendv1.BackendInternalServiceFetchArticlesByTagResponse], error) {
	return legacyStub[backendv1.BackendInternalServiceFetchArticlesByTagResponse](s, "FetchArticlesByTag", r)
}

func (s *spyLegacy) ListRecapArticles(_ context.Context, r *connect.Request[backendv1.ListRecapArticlesRequest]) (*connect.Response[backendv1.ListRecapArticlesResponse], error) {
	return legacyStub[backendv1.ListRecapArticlesResponse](s, "ListRecapArticles", r)
}

// -----------------------------------------------------------------------------
// Fakes for the two absorbed REST routes.
// -----------------------------------------------------------------------------

type fakeSystemUser struct {
	id  string
	err error
}

func (f *fakeSystemUser) GetFirstIdentityID(context.Context) (string, error) {
	return f.id, f.err
}

type fakeRecentArticles struct {
	gotInput fetch_recent_articles_usecase.FetchRecentArticlesInput
	out      *fetch_recent_articles_usecase.FetchRecentArticlesOutput
	err      error
}

func (f *fakeRecentArticles) Execute(_ context.Context, in fetch_recent_articles_usecase.FetchRecentArticlesInput) (*fetch_recent_articles_usecase.FetchRecentArticlesOutput, error) {
	f.gotInput = in
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

// -----------------------------------------------------------------------------
// Delegation
// -----------------------------------------------------------------------------

// TestDelegatesEachProcedureToItsNamesake calls every migrated procedure by
// reflection and asserts the legacy handler saw the procedure of the same name.
//
// A hand-written table would only cover the procedures someone remembered to
// list, and the mis-wiring this guards against type-checks: the adapter's
// delegate helper is generic over both message types, so pointing
// ListDeletedArticles at the legacy ListArticlesWithTags compiles cleanly and
// answers with a plausible-looking empty page.
func TestDelegatesEachProcedureToItsNamesake(t *testing.T) {
	newSvc := datahubService(t)
	handler := newTestHandler(&spyLegacy{})
	hv := reflect.ValueOf(handler)

	for i := range newSvc.Methods().Len() {
		name := string(newSvc.Methods().Get(i).Name())
		if _, absorbed := absorbedRESTProcedures[name]; absorbed {
			continue
		}

		t.Run(name, func(t *testing.T) {
			spy := &spyLegacy{}
			h := reflect.ValueOf(newTestHandler(spy))

			m := h.MethodByName(name)
			require.True(t, m.IsValid(), "adapter has no method %s", name)

			out := m.Call([]reflect.Value{
				reflect.ValueOf(context.Background()),
				zeroConnectRequest(t, m.Type().In(1)),
			})
			require.Len(t, out, 2)
			require.True(t, out[1].IsNil(), "%s returned an error: %v", name, out[1])

			assert.Equal(t, []string{name}, spy.calls,
				"%s must delegate to BackendInternalService.%s", name, name)
		})
	}
	_ = hv
}

// zeroConnectRequest builds a *connect.Request[T] carrying a zero T, given the
// reflected parameter type.
func zeroConnectRequest(t *testing.T, reqPtrType reflect.Type) reflect.Value {
	t.Helper()
	req := reflect.New(reqPtrType.Elem())
	msg := req.Elem().FieldByName("Msg")
	require.True(t, msg.IsValid(), "connect.Request has no exported Msg field")
	msg.Set(reflect.New(msg.Type().Elem()))
	return req
}

// -----------------------------------------------------------------------------
// Translation
// -----------------------------------------------------------------------------

func TestTranslatesRequestsOntoTheLegacyMessage(t *testing.T) {
	created := timestamppb.New(time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	mark := timestamppb.New(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name string
		// call invokes one adapter procedure with a fully populated request.
		call func(*testing.T, *Handler) error
		want proto.Message
	}{
		{
			name: "forward pagination carries both timestamps and the cursor",
			call: func(t *testing.T, h *Handler) error {
				_, err := h.ListArticlesWithTagsForward(context.Background(), connect.NewRequest(&datahubv1.ListArticlesWithTagsForwardRequest{
					IncrementalMark: mark,
					LastCreatedAt:   created,
					LastId:          "art-9",
					Limit:           250,
				}))
				return err
			},
			want: &backendv1.ListArticlesWithTagsForwardRequest{
				IncrementalMark: mark,
				LastCreatedAt:   created,
				LastId:          "art-9",
				Limit:           250,
			},
		},
		{
			name: "batch upsert carries nested repeated messages and floats",
			call: func(t *testing.T, h *Handler) error {
				_, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
					Items: []*datahubv1.UpsertArticleTagsRequest{
						{
							ArticleId: "art-1",
							FeedId:    "feed-1",
							Tags: []*datahubv1.TagItem{
								{Name: "go", Confidence: 0.75},
								{Name: "rust", Confidence: 0.5},
							},
						},
					},
				}))
				return err
			},
			want: &backendv1.BatchUpsertArticleTagsRequest{
				Items: []*backendv1.UpsertArticleTagsRequest{
					{
						ArticleId: "art-1",
						FeedId:    "feed-1",
						Tags: []*backendv1.TagItem{
							{Name: "go", Confidence: 0.75},
							{Name: "rust", Confidence: 0.5},
						},
					},
				},
			},
		},
		{
			name: "an explicit zero page size survives as an explicit zero",
			call: func(t *testing.T, h *Handler) error {
				_, err := h.ListRecapArticles(context.Background(), connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
					From:     "2026-07-01T00:00:00Z",
					To:       "2026-07-08T00:00:00Z",
					Page:     proto.Int32(2),
					PageSize: proto.Int32(0),
					Fields:   []string{"title"},
					LangHint: proto.String("ja"),
				}))
				return err
			},
			want: &backendv1.ListRecapArticlesRequest{
				From:     "2026-07-01T00:00:00Z",
				To:       "2026-07-08T00:00:00Z",
				Page:     proto.Int32(2),
				PageSize: proto.Int32(0),
				Fields:   []string{"title"},
				LangHint: proto.String("ja"),
			},
		},
		{
			name: "the renamed tag-cloud request keeps its field",
			call: func(t *testing.T, h *Handler) error {
				_, err := h.FetchTagCloud(context.Background(), connect.NewRequest(&datahubv1.FetchTagCloudRequest{Limit: 42}))
				return err
			},
			want: &backendv1.BackendInternalServiceFetchTagCloudRequest{Limit: 42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spyLegacy{}
			require.NoError(t, tt.call(t, newTestHandler(spy)))

			require.NotNil(t, spy.lastIn)
			assert.Truef(t, proto.Equal(tt.want, spy.lastIn),
				"legacy request differs:\n want %v\n got  %v", tt.want, spy.lastIn)
		})
	}
}

func TestTranslatesResponsesBackOntoTheDataHubMessage(t *testing.T) {
	updated := timestamppb.New(time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))

	spy := &spyLegacy{canned: &backendv1.BatchGetTagsByArticleIDsResponse{
		Items: []*backendv1.ArticleTagsEntry{
			{
				ArticleId: "art-1",
				Tags: []*backendv1.ArticleTagEntry{
					{TagName: "go", Confidence: 0.9, Source: "ml_model", UpdatedAt: updated},
				},
			},
		},
	}}

	resp, err := newTestHandler(spy).BatchGetTagsByArticleIDs(
		context.Background(),
		connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{ArticleIds: []string{"art-1"}}),
	)
	require.NoError(t, err)

	want := &datahubv1.BatchGetTagsByArticleIDsResponse{
		Items: []*datahubv1.ArticleTagsEntry{
			{
				ArticleId: "art-1",
				Tags: []*datahubv1.ArticleTagEntry{
					{TagName: "go", Confidence: 0.9, Source: "ml_model", UpdatedAt: updated},
				},
			},
		},
	}
	assert.Truef(t, proto.Equal(want, resp.Msg), "want %v got %v", want, resp.Msg)
}

// The adapter must not rewrite the legacy handler's Connect error code: a
// consumer that retries on Unavailable but not on InvalidArgument would change
// behaviour purely by moving namespace.
func TestPropagatesLegacyConnectErrorsUnchanged(t *testing.T) {
	want := connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	spy := &spyLegacy{err: want}

	_, err := newTestHandler(spy).CheckArticleExists(
		context.Background(),
		connect.NewRequest(&datahubv1.CheckArticleExistsRequest{}),
	)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "url is required")
}

// A handler built without one of its collaborators would answer some of its 26
// procedures with a nil-pointer panic on the first real request, and there is
// no honest error code for "the operator forgot to wire this" — Unimplemented
// says the procedure was retired, which is a different fact. Refusing at
// wiring time turns a DI mistake into a process that does not start
// (CLAUDE.md rule 8).
func TestNewHandlerRefusesToBuildWithAMissingDependency(t *testing.T) {
	tests := []struct {
		name           string
		legacy         backendv1connect.BackendInternalServiceHandler
		systemUser     SystemUserPort
		recentArticles RecentArticlesUsecase
	}{
		{
			name:           "no legacy handler",
			systemUser:     &fakeSystemUser{},
			recentArticles: &fakeRecentArticles{},
		},
		{
			name:           "no system user port",
			legacy:         &spyLegacy{},
			recentArticles: &fakeRecentArticles{},
		},
		{
			name:       "no recent articles usecase",
			legacy:     &spyLegacy{},
			systemUser: &fakeSystemUser{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				NewHandler(tt.legacy, tt.systemUser, tt.recentArticles, testLogger())
			})
		})
	}
}

// -----------------------------------------------------------------------------
// Absorbed REST routes (ADR-000954 D6)
// -----------------------------------------------------------------------------

func TestGetSystemUser(t *testing.T) {
	t.Run("returns the first identity id", func(t *testing.T) {
		h := newTestHandler(&spyLegacy{}, withSystemUser(&fakeSystemUser{id: "identity-1"}))

		resp, err := h.GetSystemUser(context.Background(), connect.NewRequest(&datahubv1.GetSystemUserRequest{}))
		require.NoError(t, err)
		assert.Equal(t, "identity-1", resp.Msg.UserId)
	})

	t.Run("maps a lookup failure to Internal", func(t *testing.T) {
		h := newTestHandler(&spyLegacy{}, withSystemUser(&fakeSystemUser{err: errors.New("auth-hub unreachable")}))

		_, err := h.GetSystemUser(context.Background(), connect.NewRequest(&datahubv1.GetSystemUserRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}

func TestListRecentArticles(t *testing.T) {
	articleID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	feedID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	published := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	since := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)
	until := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	newFake := func() *fakeRecentArticles {
		return &fakeRecentArticles{out: &fetch_recent_articles_usecase.FetchRecentArticlesOutput{
			Articles: []*domain.Article{{
				ID:          articleID,
				FeedID:      feedID,
				Title:       "Recent",
				URL:         "https://example.test/recent",
				PublishedAt: published,
				Tags:        []string{"go"},
			}},
			Since: since,
			Until: until,
			Count: 1,
		}}
	}

	t.Run("maps the usecase output onto the response", func(t *testing.T) {
		fake := newFake()
		h := newTestHandler(&spyLegacy{}, withRecentArticles(fake))

		resp, err := h.ListRecentArticles(context.Background(), connect.NewRequest(&datahubv1.ListRecentArticlesRequest{}))
		require.NoError(t, err)

		want := &datahubv1.ListRecentArticlesResponse{
			Articles: []*datahubv1.RecentArticleItem{{
				Id:          articleID.String(),
				Title:       "Recent",
				Url:         "https://example.test/recent",
				PublishedAt: published.Format(time.RFC3339),
				FeedId:      feedID.String(),
				Tags:        []string{"go"},
			}},
			Since: since.Format(time.RFC3339),
			Until: until.Format(time.RFC3339),
			Count: 1,
		}
		assert.Truef(t, proto.Equal(want, resp.Msg), "want %v got %v", want, resp.Msg)
	})

	// The REST route these replace defaults within_hours to 24 and limit to
	// 100, and treats limit=0 as "no count limit". proto3 cannot tell an unset
	// int32 from a zero one, which is why both fields are optional; this is
	// the test that the distinction survives.
	t.Run("applies the REST defaults and preserves an explicit zero limit", func(t *testing.T) {
		tests := []struct {
			name string
			req  *datahubv1.ListRecentArticlesRequest
			want fetch_recent_articles_usecase.FetchRecentArticlesInput
		}{
			{
				name: "unset",
				req:  &datahubv1.ListRecentArticlesRequest{},
				want: fetch_recent_articles_usecase.FetchRecentArticlesInput{WithinHours: 24, Limit: 100},
			},
			{
				name: "explicit zero limit means no count limit",
				req:  &datahubv1.ListRecentArticlesRequest{Limit: proto.Int32(0)},
				want: fetch_recent_articles_usecase.FetchRecentArticlesInput{WithinHours: 24, Limit: 0},
			},
			{
				name: "explicit values pass through",
				req:  &datahubv1.ListRecentArticlesRequest{WithinHours: proto.Int32(72), Limit: proto.Int32(5)},
				want: fetch_recent_articles_usecase.FetchRecentArticlesInput{WithinHours: 72, Limit: 5},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				fake := newFake()
				h := newTestHandler(&spyLegacy{}, withRecentArticles(fake))

				_, err := h.ListRecentArticles(context.Background(), connect.NewRequest(tt.req))
				require.NoError(t, err)
				assert.Equal(t, tt.want, fake.gotInput)
			})
		}
	})

	t.Run("rejects the values the REST route rejected", func(t *testing.T) {
		tests := []struct {
			name string
			req  *datahubv1.ListRecentArticlesRequest
		}{
			{name: "non-positive within_hours", req: &datahubv1.ListRecentArticlesRequest{WithinHours: proto.Int32(0)}},
			{name: "negative within_hours", req: &datahubv1.ListRecentArticlesRequest{WithinHours: proto.Int32(-1)}},
			{name: "negative limit", req: &datahubv1.ListRecentArticlesRequest{Limit: proto.Int32(-1)}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := newTestHandler(&spyLegacy{}, withRecentArticles(newFake()))

				_, err := h.ListRecentArticles(context.Background(), connect.NewRequest(tt.req))
				require.Error(t, err)
				assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			})
		}
	})
}
