package global_search

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"alt/domain"
	searchv2 "alt/gen/proto/alt/search/v2"
	"alt/orchestrator/usecase/global_search_usecase"
	"alt/utils/logger"
)

type stubArticleSearchPort struct {
	section    *domain.ArticleSearchSection
	err        error
	lastQuery  string
	lastUserID string
	lastLimit  int
}

func (s *stubArticleSearchPort) SearchArticlesForGlobal(_ context.Context, query string, userID string, limit int) (*domain.ArticleSearchSection, error) {
	s.lastQuery = query
	s.lastUserID = userID
	s.lastLimit = limit
	return s.section, s.err
}

type stubRecapSearchPort struct {
	section   *domain.RecapSearchSection
	err       error
	lastQuery string
	lastLimit int
}

func (s *stubRecapSearchPort) SearchRecapsForGlobal(_ context.Context, query string, limit int) (*domain.RecapSearchSection, error) {
	s.lastQuery = query
	s.lastLimit = limit
	return s.section, s.err
}

type stubTagSearchPort struct {
	section    *domain.TagSearchSection
	err        error
	lastPrefix string
	lastLimit  int
}

func (s *stubTagSearchPort) SearchTagsByPrefix(_ context.Context, prefix string, limit int) (*domain.TagSearchSection, error) {
	s.lastPrefix = prefix
	s.lastLimit = limit
	return s.section, s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func authenticatedCtx(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		TenantID:  userID,
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestSearchEverything_EmptyQuery_ReturnsInvalidArgument(t *testing.T) {
	logger.InitLogger()
	tracer := noop.NewTracerProvider().Tracer(global_search_usecase.TracerName)
	uc := global_search_usecase.NewGlobalSearchUsecase(
		&stubArticleSearchPort{},
		&stubRecapSearchPort{},
		&stubTagSearchPort{},
		tracer,
	)
	h := NewHandler(uc, testLogger())

	resp, err := h.SearchEverything(
		authenticatedCtx(uuid.New()),
		connect.NewRequest(&searchv2.SearchEverythingRequest{Query: ""}),
	)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestSearchEverything_UnauthenticatedContext_ReturnsError(t *testing.T) {
	logger.InitLogger()
	tracer := noop.NewTracerProvider().Tracer(global_search_usecase.TracerName)
	uc := global_search_usecase.NewGlobalSearchUsecase(
		&stubArticleSearchPort{},
		&stubRecapSearchPort{},
		&stubTagSearchPort{},
		tracer,
	)
	h := NewHandler(uc, testLogger())

	resp, err := h.SearchEverything(
		context.Background(),
		connect.NewRequest(&searchv2.SearchEverythingRequest{Query: "distributed systems"}),
	)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestSearchEverything_MapsArticlesRecapsAndTagsSections(t *testing.T) {
	logger.InitLogger()
	tracer := noop.NewTracerProvider().Tracer(global_search_usecase.TracerName)
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	articlePort := &stubArticleSearchPort{
		section: &domain.ArticleSearchSection{
			Hits: []domain.GlobalArticleHit{
				{
					ID:            "art-1",
					Title:         "Observability in Go",
					Snippet:       "OpenTelemetry tracing patterns",
					Link:          "https://example.com/art-1",
					Tags:          []string{"go", "otel"},
					MatchedFields: []string{"title", "content"},
				},
				{
					ID:            "art-2",
					Title:         "Nil Slice Normalization",
					Snippet:       "Defaults nil slices to empty",
					Link:          "https://example.com/art-2",
					Tags:          nil,
					MatchedFields: nil,
				},
			},
			EstimatedTotal: 12,
			HasMore:        true,
		},
	}
	recapPort := &stubRecapSearchPort{
		section: &domain.RecapSearchSection{
			Hits: []domain.GlobalRecapHit{
				{
					ID:         "recap-1",
					JobID:      "job-99",
					Genre:      "software_engineering",
					Summary:    "Weekly Go ecosystem highlights",
					TopTerms:   []string{"golang", "connectrpc"},
					Tags:       []string{"backend"},
					WindowDays: 7,
					ExecutedAt: "2026-10-20T00:00:00Z",
				},
				{
					ID:         "recap-2",
					JobID:      "job-100",
					Genre:      "ai",
					Summary:    "Nil slices normalized",
					TopTerms:   nil,
					Tags:       nil,
					WindowDays: 3,
					ExecutedAt: "2026-10-21T00:00:00Z",
				},
			},
			EstimatedTotal: 2,
			HasMore:        false,
		},
	}
	tagPort := &stubTagSearchPort{
		section: &domain.TagSearchSection{
			Hits: []domain.GlobalTagHit{
				{TagName: "golang", ArticleCount: 42},
				{TagName: "opentelemetry", ArticleCount: 15},
			},
			Total: 2,
		},
	}

	uc := global_search_usecase.NewGlobalSearchUsecase(articlePort, recapPort, tagPort, tracer)
	h := NewHandler(uc, testLogger())

	resp, err := h.SearchEverything(
		authenticatedCtx(userID),
		connect.NewRequest(&searchv2.SearchEverythingRequest{
			Query:        "golang",
			ArticleLimit: 7,
			RecapLimit:   4,
			TagLimit:     12,
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)

	msg := resp.Msg
	assert.Equal(t, "golang", msg.Query)
	assert.Empty(t, msg.DegradedSections)
	_, parseErr := time.Parse(time.RFC3339, msg.SearchedAt)
	assert.NoError(t, parseErr, "SearchedAt must be formatted as RFC3339")

	// Verify port arguments
	assert.Equal(t, userID.String(), articlePort.lastUserID)
	assert.Equal(t, 7, articlePort.lastLimit)
	assert.Equal(t, 4, recapPort.lastLimit)
	assert.Equal(t, 12, tagPort.lastLimit)

	// Verify ArticleSection mapping
	require.NotNil(t, msg.ArticleSection)
	assert.Equal(t, int64(12), msg.ArticleSection.EstimatedTotal)
	assert.True(t, msg.ArticleSection.HasMore)
	require.Len(t, msg.ArticleSection.Hits, 2)
	assert.Equal(t, "art-1", msg.ArticleSection.Hits[0].Id)
	assert.Equal(t, "Observability in Go", msg.ArticleSection.Hits[0].Title)
	assert.Equal(t, "OpenTelemetry tracing patterns", msg.ArticleSection.Hits[0].Snippet)
	assert.Equal(t, "https://example.com/art-1", msg.ArticleSection.Hits[0].Link)
	assert.Equal(t, []string{"go", "otel"}, msg.ArticleSection.Hits[0].Tags)
	assert.Equal(t, []string{"title", "content"}, msg.ArticleSection.Hits[0].MatchedFields)
	assert.Empty(t, msg.ArticleSection.Hits[1].Tags)
	assert.Empty(t, msg.ArticleSection.Hits[1].MatchedFields)

	// Verify RecapSection mapping
	require.NotNil(t, msg.RecapSection)
	assert.Equal(t, int64(2), msg.RecapSection.EstimatedTotal)
	assert.False(t, msg.RecapSection.HasMore)
	require.Len(t, msg.RecapSection.Hits, 2)
	assert.Equal(t, "recap-1", msg.RecapSection.Hits[0].Id)
	assert.Equal(t, "job-99", msg.RecapSection.Hits[0].JobId)
	assert.Equal(t, "software_engineering", msg.RecapSection.Hits[0].Genre)
	assert.Equal(t, "Weekly Go ecosystem highlights", msg.RecapSection.Hits[0].Summary)
	assert.Equal(t, []string{"golang", "connectrpc"}, msg.RecapSection.Hits[0].TopTerms)
	assert.Equal(t, []string{"backend"}, msg.RecapSection.Hits[0].Tags)
	assert.Equal(t, int32(7), msg.RecapSection.Hits[0].WindowDays)
	assert.Equal(t, "2026-10-20T00:00:00Z", msg.RecapSection.Hits[0].ExecutedAt)
	assert.Empty(t, msg.RecapSection.Hits[1].TopTerms)
	assert.Empty(t, msg.RecapSection.Hits[1].Tags)

	// Verify TagSection mapping
	require.NotNil(t, msg.TagSection)
	assert.Equal(t, int64(2), msg.TagSection.Total)
	require.Len(t, msg.TagSection.Hits, 2)
	assert.Equal(t, "golang", msg.TagSection.Hits[0].TagName)
	assert.Equal(t, int32(42), msg.TagSection.Hits[0].ArticleCount)
	assert.Equal(t, "opentelemetry", msg.TagSection.Hits[1].TagName)
	assert.Equal(t, int32(15), msg.TagSection.Hits[1].ArticleCount)
}

func TestSearchEverything_PartialFailure_ReturnsDegradedSections(t *testing.T) {
	logger.InitLogger()
	tracer := noop.NewTracerProvider().Tracer(global_search_usecase.TracerName)

	uc := global_search_usecase.NewGlobalSearchUsecase(
		&stubArticleSearchPort{err: errors.New("meilisearch timeout")},
		&stubRecapSearchPort{
			section: &domain.RecapSearchSection{
				Hits:           []domain.GlobalRecapHit{{ID: "recap-1", Genre: "tech"}},
				EstimatedTotal: 1,
			},
		},
		&stubTagSearchPort{
			section: &domain.TagSearchSection{
				Hits:  []domain.GlobalTagHit{{TagName: "tech", ArticleCount: 8}},
				Total: 1,
			},
		},
		tracer,
	)
	h := NewHandler(uc, testLogger())

	resp, err := h.SearchEverything(
		authenticatedCtx(uuid.New()),
		connect.NewRequest(&searchv2.SearchEverythingRequest{Query: "tech"}),
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, []string{"articles"}, resp.Msg.DegradedSections)
	assert.Nil(t, resp.Msg.ArticleSection)
	require.NotNil(t, resp.Msg.RecapSection)
	assert.Len(t, resp.Msg.RecapSection.Hits, 1)
	require.NotNil(t, resp.Msg.TagSection)
	assert.Len(t, resp.Msg.TagSection.Hits, 1)
}

func TestSearchEverything_AllSectionsFail_ReturnsInternalError(t *testing.T) {
	logger.InitLogger()
	tracer := noop.NewTracerProvider().Tracer(global_search_usecase.TracerName)

	uc := global_search_usecase.NewGlobalSearchUsecase(
		&stubArticleSearchPort{err: errors.New("articles down")},
		&stubRecapSearchPort{err: errors.New("recaps down")},
		&stubTagSearchPort{err: errors.New("tags down")},
		tracer,
	)
	h := NewHandler(uc, testLogger())

	resp, err := h.SearchEverything(
		authenticatedCtx(uuid.New()),
		connect.NewRequest(&searchv2.SearchEverythingRequest{Query: "tech"}),
	)

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
