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
	"google.golang.org/protobuf/reflect/protoreflect"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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
// Surface
// -----------------------------------------------------------------------------

// TestHandlerImplementsEveryDataHubProcedure walks the service descriptor
// rather than listing the procedures.
//
// The compiler already refuses a Handler that is missing a method — the
// DataHubServiceHandler assertion in handler.go sees to that. What it cannot
// see is a procedure added to the proto and then left off the Go interface by
// a stale `buf generate`, which is exactly the state ADR-000954's namespace
// move passed through twice. Comparing the descriptor against the methods the
// handler actually has catches that without anyone maintaining a list.
func TestHandlerImplementsEveryDataHubProcedure(t *testing.T) {
	svcs := datahubv1.File_alt_datahub_v1_datahub_proto.Services()

	var svc protoreflect.ServiceDescriptor
	for i := range svcs.Len() {
		if svcs.Get(i).Name() == "DataHubService" {
			svc = svcs.Get(i)
		}
	}
	require.NotNil(t, svc, "alt.datahub.v1.DataHubService not found in the generated file descriptor")

	h := reflect.ValueOf(newTestHandler())
	for i := range svc.Methods().Len() {
		name := string(svc.Methods().Get(i).Name())
		t.Run(name, func(t *testing.T) {
			assert.Truef(t, h.MethodByName(name).IsValid(), "Handler has no method %s", name)
		})
	}
}

// withSystemUser / withRecentArticles override a required dependency after
// construction. They are test-local rather than exported options because
// NewHandler takes both positionally on purpose — see its comment.
func withSystemUser(p SystemUserPort) HandlerOption {
	return func(h *Handler) { h.systemUser = p }
}

func withRecentArticles(u RecentArticlesUsecase) HandlerOption {
	return func(h *Handler) { h.recentArticles = u }
}

// newTestHandler builds a Handler with the two required dependencies satisfied
// and every optional port left unwired, because NewHandler refuses to build
// otherwise. Tests that care about one dependency override it.
func newTestHandler(opts ...HandlerOption) *Handler {
	return NewHandler(nil, nil, nil, nil, nil,
		&fakeSystemUser{},
		&fakeRecentArticles{out: &fetch_recent_articles_usecase.FetchRecentArticlesOutput{}},
		testLogger(), opts...)
}

// A handler built without one of its required collaborators would answer two
// of its 26 procedures with a nil-pointer panic on the first real request, and
// there is no honest error code for "the operator forgot to wire this" —
// Unimplemented says the procedure was retired, which is a different fact.
// Refusing at wiring time turns a DI mistake into a process that does not
// start (CLAUDE.md rule 8).
func TestNewHandlerRefusesToBuildWithAMissingDependency(t *testing.T) {
	tests := []struct {
		name           string
		systemUser     SystemUserPort
		recentArticles RecentArticlesUsecase
	}{
		{
			name:           "no system user port",
			recentArticles: &fakeRecentArticles{},
		},
		{
			name:       "no recent articles usecase",
			systemUser: &fakeSystemUser{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				NewHandler(nil, nil, nil, nil, nil, tt.systemUser, tt.recentArticles, testLogger())
			})
		})
	}
}

// -----------------------------------------------------------------------------
// Absorbed REST routes (ADR-000954 D6)
// -----------------------------------------------------------------------------

func TestGetSystemUser(t *testing.T) {
	t.Run("returns the first identity id", func(t *testing.T) {
		h := newTestHandler(withSystemUser(&fakeSystemUser{id: "identity-1"}))

		resp, err := h.GetSystemUser(context.Background(), connect.NewRequest(&datahubv1.GetSystemUserRequest{}))
		require.NoError(t, err)
		assert.Equal(t, "identity-1", resp.Msg.UserId)
	})

	t.Run("maps a lookup failure to Internal", func(t *testing.T) {
		h := newTestHandler(withSystemUser(&fakeSystemUser{err: errors.New("auth-hub unreachable")}))

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
		h := newTestHandler(withRecentArticles(newFake()))

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
				h := newTestHandler(withRecentArticles(fake))

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
				h := newTestHandler(withRecentArticles(newFake()))

				_, err := h.ListRecentArticles(context.Background(), connect.NewRequest(tt.req))
				require.Error(t, err)
				assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			})
		}
	})
}
