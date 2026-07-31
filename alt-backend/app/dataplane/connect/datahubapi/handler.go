// Package datahubapi implements alt.datahub.v1.DataHubService, the namespace
// alt-data-hub serves under its own name (ADR-000954 D3/D7).
//
// During Wave 2 alt-data-hub answers on two namespaces at once. This package
// holds no data-access logic of its own for the 24 procedures it inherits: it
// re-encodes each request onto the equivalent services.backend.v1 message and
// hands it to the existing BackendInternalService handler. Duplicating the
// logic instead would mean two implementations of the same transaction
// boundaries diverging over however many PRs Wave 2-B takes, and the
// divergence would show up as one peer seeing different behaviour from
// another purely because of when it was migrated.
//
// The re-encoding is a marshal/unmarshal round trip rather than field-by-field
// assignment. That is deliberate and it is the cheap part of the design:
//
//   - It cannot mistranslate a field. Copying ~200 fields by hand across 48
//     messages is exactly the kind of work where one transposed line survives
//     review, and that line would silently corrupt a peer's data during the
//     one wave where nobody is looking at these payloads closely.
//   - It states the invariant Wave 2-B depends on — the two message trees are
//     the same wire schema — as executable code rather than as a comment.
//   - wirecompat_test.go proves the round trip is lossless by walking both
//     descriptor sets, so the case the round trip cannot detect (a field
//     present on one side only, which protobuf tolerates by design) fails the
//     build instead.
//
// The cost is one extra marshal and unmarshal per call on a service-to-service
// path that already pays for HTTP and TLS. Wave 2-C deletes the legacy
// namespace; at that point the internal handler moves onto these message types
// and this package, along with the round trip, goes away.
//
// Two procedures are not delegated: GetSystemUser and ListRecentArticles are
// the /v1/internal REST routes that ADR-000954 D6 folds into the Connect
// surface, and they call the same usecases the REST handlers do.
package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	datahubv1 "alt/gen/proto/alt/datahub/v1"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/gen/proto/services/backend/v1/backendv1connect"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/utils/safeconv"
)

// Defaults copied from the REST routes ADR-000954 D6 absorbs. They live here
// rather than being left to the usecase because the RPC has to distinguish
// "caller omitted the field" from "caller sent zero", and only the delivery
// layer can see that difference.
const (
	defaultRecentWithinHours = 24
	defaultRecentLimit       = 100
)

// SystemUserPort resolves the system identity that service-to-service callers
// act as. Backed by the Kratos client; declared here so the handler depends on
// the one method it uses.
type SystemUserPort interface {
	GetFirstIdentityID(ctx context.Context) (string, error)
}

// RecentArticlesUsecase is the read behind GET /v1/internal/articles/recent.
type RecentArticlesUsecase interface {
	Execute(ctx context.Context, in fetch_recent_articles_usecase.FetchRecentArticlesInput) (*fetch_recent_articles_usecase.FetchRecentArticlesOutput, error)
}

// Handler implements DataHubServiceHandler.
type Handler struct {
	// legacy is the BackendInternalService implementation every migrated
	// procedure forwards to. Required — see NewHandler.
	legacy backendv1connect.BackendInternalServiceHandler

	// Absorbed REST routes (ADR-000954 D6).
	systemUser     SystemUserPort
	recentArticles RecentArticlesUsecase

	logger *slog.Logger
}

var _ datahubv1connect.DataHubServiceHandler = (*Handler)(nil)

// NewHandler builds the adapter. Every dependency is required and a missing
// one panics.
//
// There are no functional options here on purpose. None of the three is a
// feature that can be switched off: DataHubService advertises 26 procedures
// and a handler missing a dependency would answer some of them with a nil
// dereference on the first real request. Returning Unimplemented instead
// would be worse, because that is also what a genuinely retired procedure
// would answer, and CLAUDE.md rule 8 exists because those two states became
// indistinguishable once before (ADR-000928). Refusing at construction makes
// a DI mistake a process that does not start.
//
// Note for callers holding concrete types: a nil *T stored in one of these
// interfaces is not nil as an interface value and will not be caught here.
// The composition root checks its own fields before calling — see
// connect/v2/datahub/server.go.
func NewHandler(
	legacy backendv1connect.BackendInternalServiceHandler,
	systemUser SystemUserPort,
	recentArticles RecentArticlesUsecase,
	logger *slog.Logger,
) *Handler {
	switch {
	case legacy == nil:
		panic("datahubapi: BackendInternalService handler is required — " +
			"DataHubService delegates every migrated procedure to it")
	case systemUser == nil:
		panic("datahubapi: SystemUserPort is required — " +
			"DataHubService.GetSystemUser replaces GET /v1/internal/system-user")
	case recentArticles == nil:
		panic("datahubapi: RecentArticlesUsecase is required — " +
			"DataHubService.ListRecentArticles replaces GET /v1/internal/articles/recent")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		legacy:         legacy,
		systemUser:     systemUser,
		recentArticles: recentArticles,
		logger:         logger,
	}
}

// -----------------------------------------------------------------------------
// Re-encoding
// -----------------------------------------------------------------------------

// protoPtr constrains PT to *T where *T is a generated protobuf message. It
// lets the helpers below allocate a destination message from its value type,
// which is what the connect.Request[T] / connect.Response[T] shapes hand us.
type protoPtr[T any] interface {
	*T
	proto.Message
}

// recode re-encodes src as Dst by round-tripping it through the protobuf wire
// format. Lossless exactly when the two descriptors agree on every field
// number, type and cardinality — the property wirecompat_test.go enforces.
func recode[Dst any, PDst protoPtr[Dst]](src proto.Message) (PDst, error) {
	raw, err := proto.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", src.ProtoReflect().Descriptor().FullName(), err)
	}
	dst := PDst(new(Dst))
	if err := proto.Unmarshal(raw, dst); err != nil {
		return nil, fmt.Errorf("unmarshal %s into %s: %w",
			src.ProtoReflect().Descriptor().FullName(),
			dst.ProtoReflect().Descriptor().FullName(), err)
	}
	return dst, nil
}

// delegate forwards one DataHubService procedure to its BackendInternalService
// namesake, translating the request on the way in and the response on the way
// out. Errors pass through untouched so a consumer sees the same Connect code
// on either namespace.
func delegate[
	DResp any, PDResp protoPtr[DResp],
	DReq any, PDReq protoPtr[DReq],
	LReq any, PLReq protoPtr[LReq],
	LResp any, PLResp protoPtr[LResp],
](
	ctx context.Context,
	req *connect.Request[DReq],
	call func(context.Context, *connect.Request[LReq]) (*connect.Response[LResp], error),
) (*connect.Response[DResp], error) {
	legacyMsg, err := recode[LReq, PLReq](PDReq(req.Msg))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	legacyReq := connect.NewRequest((*LReq)(legacyMsg))
	copyHeader(legacyReq.Header(), req.Header())

	legacyResp, err := call(ctx, legacyReq)
	if err != nil {
		return nil, err
	}

	msg, err := recode[DResp, PDResp](PLResp(legacyResp.Msg))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse((*DResp)(msg))
	copyHeader(resp.Header(), legacyResp.Header())
	copyHeader(resp.Trailer(), legacyResp.Trailer())
	return resp, nil
}

func copyHeader(dst, src http.Header) {
	if dst == nil {
		return
	}
	for k, values := range src {
		for _, v := range values {
			dst.Add(k, v)
		}
	}
}

// -----------------------------------------------------------------------------
// Migrated procedures — Origin comments live on the proto definitions.
// -----------------------------------------------------------------------------

func (h *Handler) ListArticlesWithTags(ctx context.Context, req *connect.Request[datahubv1.ListArticlesWithTagsRequest]) (*connect.Response[datahubv1.ListArticlesWithTagsResponse], error) {
	return delegate[datahubv1.ListArticlesWithTagsResponse](ctx, req, h.legacy.ListArticlesWithTags)
}

func (h *Handler) ListArticlesWithTagsForward(ctx context.Context, req *connect.Request[datahubv1.ListArticlesWithTagsForwardRequest]) (*connect.Response[datahubv1.ListArticlesWithTagsForwardResponse], error) {
	return delegate[datahubv1.ListArticlesWithTagsForwardResponse](ctx, req, h.legacy.ListArticlesWithTagsForward)
}

func (h *Handler) ListDeletedArticles(ctx context.Context, req *connect.Request[datahubv1.ListDeletedArticlesRequest]) (*connect.Response[datahubv1.ListDeletedArticlesResponse], error) {
	return delegate[datahubv1.ListDeletedArticlesResponse](ctx, req, h.legacy.ListDeletedArticles)
}

func (h *Handler) GetLatestArticleTimestamp(ctx context.Context, req *connect.Request[datahubv1.GetLatestArticleTimestampRequest]) (*connect.Response[datahubv1.GetLatestArticleTimestampResponse], error) {
	return delegate[datahubv1.GetLatestArticleTimestampResponse](ctx, req, h.legacy.GetLatestArticleTimestamp)
}

func (h *Handler) GetArticleByID(ctx context.Context, req *connect.Request[datahubv1.GetArticleByIDRequest]) (*connect.Response[datahubv1.GetArticleByIDResponse], error) {
	return delegate[datahubv1.GetArticleByIDResponse](ctx, req, h.legacy.GetArticleByID)
}

func (h *Handler) CheckArticleExists(ctx context.Context, req *connect.Request[datahubv1.CheckArticleExistsRequest]) (*connect.Response[datahubv1.CheckArticleExistsResponse], error) {
	return delegate[datahubv1.CheckArticleExistsResponse](ctx, req, h.legacy.CheckArticleExists)
}

func (h *Handler) CreateArticle(ctx context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
	return delegate[datahubv1.CreateArticleResponse](ctx, req, h.legacy.CreateArticle)
}

func (h *Handler) SaveArticleSummary(ctx context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error) {
	return delegate[datahubv1.SaveArticleSummaryResponse](ctx, req, h.legacy.SaveArticleSummary)
}

func (h *Handler) GetArticleContent(ctx context.Context, req *connect.Request[datahubv1.GetArticleContentRequest]) (*connect.Response[datahubv1.GetArticleContentResponse], error) {
	return delegate[datahubv1.GetArticleContentResponse](ctx, req, h.legacy.GetArticleContent)
}

func (h *Handler) GetFeedID(ctx context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
	return delegate[datahubv1.GetFeedIDResponse](ctx, req, h.legacy.GetFeedID)
}

func (h *Handler) ListFeedURLs(ctx context.Context, req *connect.Request[datahubv1.ListFeedURLsRequest]) (*connect.Response[datahubv1.ListFeedURLsResponse], error) {
	return delegate[datahubv1.ListFeedURLsResponse](ctx, req, h.legacy.ListFeedURLs)
}

func (h *Handler) UpsertArticleTags(ctx context.Context, req *connect.Request[datahubv1.UpsertArticleTagsRequest]) (*connect.Response[datahubv1.UpsertArticleTagsResponse], error) {
	return delegate[datahubv1.UpsertArticleTagsResponse](ctx, req, h.legacy.UpsertArticleTags)
}

func (h *Handler) BatchUpsertArticleTags(ctx context.Context, req *connect.Request[datahubv1.BatchUpsertArticleTagsRequest]) (*connect.Response[datahubv1.BatchUpsertArticleTagsResponse], error) {
	return delegate[datahubv1.BatchUpsertArticleTagsResponse](ctx, req, h.legacy.BatchUpsertArticleTags)
}

func (h *Handler) ListUntaggedArticles(ctx context.Context, req *connect.Request[datahubv1.ListUntaggedArticlesRequest]) (*connect.Response[datahubv1.ListUntaggedArticlesResponse], error) {
	return delegate[datahubv1.ListUntaggedArticlesResponse](ctx, req, h.legacy.ListUntaggedArticles)
}

func (h *Handler) BatchGetTagsByArticleIDs(ctx context.Context, req *connect.Request[datahubv1.BatchGetTagsByArticleIDsRequest]) (*connect.Response[datahubv1.BatchGetTagsByArticleIDsResponse], error) {
	return delegate[datahubv1.BatchGetTagsByArticleIDsResponse](ctx, req, h.legacy.BatchGetTagsByArticleIDs)
}

func (h *Handler) DeleteArticleSummary(ctx context.Context, req *connect.Request[datahubv1.DeleteArticleSummaryRequest]) (*connect.Response[datahubv1.DeleteArticleSummaryResponse], error) {
	return delegate[datahubv1.DeleteArticleSummaryResponse](ctx, req, h.legacy.DeleteArticleSummary)
}

func (h *Handler) CheckArticleSummaryExists(ctx context.Context, req *connect.Request[datahubv1.CheckArticleSummaryExistsRequest]) (*connect.Response[datahubv1.CheckArticleSummaryExistsResponse], error) {
	return delegate[datahubv1.CheckArticleSummaryExistsResponse](ctx, req, h.legacy.CheckArticleSummaryExists)
}

func (h *Handler) FindArticlesWithSummaries(ctx context.Context, req *connect.Request[datahubv1.FindArticlesWithSummariesRequest]) (*connect.Response[datahubv1.FindArticlesWithSummariesResponse], error) {
	return delegate[datahubv1.FindArticlesWithSummariesResponse](ctx, req, h.legacy.FindArticlesWithSummaries)
}

func (h *Handler) ListUnsummarizedArticles(ctx context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
	return delegate[datahubv1.ListUnsummarizedArticlesResponse](ctx, req, h.legacy.ListUnsummarizedArticles)
}

func (h *Handler) HasUnsummarizedArticles(ctx context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
	return delegate[datahubv1.HasUnsummarizedArticlesResponse](ctx, req, h.legacy.HasUnsummarizedArticles)
}

func (h *Handler) GetEmptyFeedID(ctx context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
	return delegate[datahubv1.GetEmptyFeedIDResponse](ctx, req, h.legacy.GetEmptyFeedID)
}

func (h *Handler) FetchTagCloud(ctx context.Context, req *connect.Request[datahubv1.FetchTagCloudRequest]) (*connect.Response[datahubv1.FetchTagCloudResponse], error) {
	return delegate[datahubv1.FetchTagCloudResponse](ctx, req, h.legacy.FetchTagCloud)
}

func (h *Handler) FetchArticlesByTag(ctx context.Context, req *connect.Request[datahubv1.FetchArticlesByTagRequest]) (*connect.Response[datahubv1.FetchArticlesByTagResponse], error) {
	return delegate[datahubv1.FetchArticlesByTagResponse](ctx, req, h.legacy.FetchArticlesByTag)
}

func (h *Handler) ListRecapArticles(ctx context.Context, req *connect.Request[datahubv1.ListRecapArticlesRequest]) (*connect.Response[datahubv1.ListRecapArticlesResponse], error) {
	return delegate[datahubv1.ListRecapArticlesResponse](ctx, req, h.legacy.ListRecapArticles)
}

// -----------------------------------------------------------------------------
// Absorbed REST routes (ADR-000954 D6)
// -----------------------------------------------------------------------------

// GetSystemUser replaces GET /v1/internal/system-user.
func (h *Handler) GetSystemUser(ctx context.Context, _ *connect.Request[datahubv1.GetSystemUserRequest]) (*connect.Response[datahubv1.GetSystemUserResponse], error) {
	userID, err := h.systemUser.GetFirstIdentityID(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetSystemUser failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch system user: %w", err))
	}

	return connect.NewResponse(&datahubv1.GetSystemUserResponse{UserId: userID}), nil
}

// ListRecentArticles replaces GET /v1/internal/articles/recent.
//
// The REST route answered 400 for a non-positive within_hours and for a
// negative limit, and treated limit=0 as "time window only". Both behaviours
// are reproduced here rather than deferred to the usecase, which clamps
// silently: a caller that sends nonsense should hear about it during the wave
// where it is switching protocols, not receive a quietly different window.
func (h *Handler) ListRecentArticles(ctx context.Context, req *connect.Request[datahubv1.ListRecentArticlesRequest]) (*connect.Response[datahubv1.ListRecentArticlesResponse], error) {
	withinHours := defaultRecentWithinHours
	if v := req.Msg.WithinHours; v != nil {
		if *v <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("within_hours must be positive"))
		}
		withinHours = int(*v)
	}

	limit := defaultRecentLimit
	if v := req.Msg.Limit; v != nil {
		if *v < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("limit must not be negative"))
		}
		limit = int(*v)
	}

	out, err := h.recentArticles.Execute(ctx, fetch_recent_articles_usecase.FetchRecentArticlesInput{
		WithinHours: withinHours,
		Limit:       limit,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "ListRecentArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch recent articles: %w", err))
	}

	items := make([]*datahubv1.RecentArticleItem, len(out.Articles))
	for i, a := range out.Articles {
		items[i] = &datahubv1.RecentArticleItem{
			Id:          a.ID.String(),
			Title:       a.Title,
			Url:         a.URL,
			PublishedAt: a.PublishedAt.Format(time.RFC3339),
			FeedId:      a.FeedID.String(),
			Tags:        a.Tags,
		}
	}

	return connect.NewResponse(&datahubv1.ListRecentArticlesResponse{
		Articles: items,
		Since:    out.Since.Format(time.RFC3339),
		Until:    out.Until.Format(time.RFC3339),
		Count:    safeconv.Int32(out.Count),
	}), nil
}
