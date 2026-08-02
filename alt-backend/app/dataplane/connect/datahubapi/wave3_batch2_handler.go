package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WithWave3Batch2Capabilities wires the article capabilities ADR-000954
// Wave 3 batch 2 moved out of alt-backend (catalog §2.B / §2.C / §2.N).
//
// Nil panics, for the reason WithWave3Capabilities gives at length: after this
// batch these procedures are alt-backend's only route to the articles table.
// A data hub that started with a nil article read port would answer
// GetArticleByURL with Unimplemented — indistinguishable from a retired
// procedure — and the article page would report "not found" for every article
// in the database while every health check stayed green (CLAUDE.md rule 8,
// ADR-000928).
//
// SaveArticleHead is not here. It is catalog §2.B, but it writes article_heads
// and is served through the OG image port WithWave3Capabilities already wires:
// one table, one port.
func WithWave3Batch2Capabilities(
	articleWrite datahub_capability_port.ArticleWritePort,
	articleRead datahub_capability_port.ArticleReadPort,
	knowledgeBackfill datahub_capability_port.KnowledgeBackfillPort,
) HandlerOption {
	switch {
	case articleWrite == nil:
		panic("datahubapi: ArticleWritePort is required — alt-backend has no other route to the articles upsert and its outbox row")
	case articleRead == nil:
		panic("datahubapi: ArticleReadPort is required — every article-serving surface reads through it")
	case knowledgeBackfill == nil:
		panic("datahubapi: KnowledgeBackfillPort is required — the knowledge backfill jobs have no other route to historic articles")
	}

	return func(h *Handler) {
		h.articleWrite = articleWrite
		h.articleRead = articleRead
		h.knowledgeBackfill = knowledgeBackfill
	}
}

// ---------------------------------------------------------------------------
// §2.B Article writes
// ---------------------------------------------------------------------------

// ArchiveArticle upserts the article and appends its outbox row in one
// transaction.
//
// user_id is required and must parse. The driver refuses the zero UUID, but
// rejecting it here as well means a caller that omitted the field learns so
// from an InvalidArgument rather than from an Internal that hides a validation
// failure behind a database-shaped error.
func (h *Handler) ArchiveArticle(ctx context.Context, req *connect.Request[datahubv1.ArchiveArticleRequest]) (*connect.Response[datahubv1.ArchiveArticleResponse], error) {
	if req.Msg.GetUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.GetContent() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("content is required"))
	}

	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	articleID, created, err := h.articleWrite.Archive(ctx, req.Msg.GetUrl(), req.Msg.GetTitle(), req.Msg.GetContent(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ArchiveArticle failed", "error", err, "url", req.Msg.GetUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to archive article"))
	}

	return connect.NewResponse(&datahubv1.ArchiveArticleResponse{
		ArticleId: articleID,
		Created:   created,
	}), nil
}

// SaveArticleHead upserts the scraped head. Served by the OG image port —
// see WithWave3Batch2Capabilities.
func (h *Handler) SaveArticleHead(ctx context.Context, req *connect.Request[datahubv1.SaveArticleHeadRequest]) (*connect.Response[datahubv1.SaveArticleHeadResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	if err := h.ogImage.SaveArticleHead(ctx, req.Msg.GetArticleId(), req.Msg.GetHeadHtml(), req.Msg.GetOgImageUrl()); err != nil {
		h.logger.ErrorContext(ctx, "SaveArticleHead failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save article head"))
	}
	return connect.NewResponse(&datahubv1.SaveArticleHeadResponse{}), nil
}

// ---------------------------------------------------------------------------
// §2.C Article reads
// ---------------------------------------------------------------------------

func (h *Handler) GetArticleByURL(ctx context.Context, req *connect.Request[datahubv1.GetArticleByURLRequest]) (*connect.Response[datahubv1.GetArticleByURLResponse], error) {
	if req.Msg.GetUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}

	userID, err := optionalUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	article, err := h.articleRead.GetByURL(ctx, req.Msg.GetUrl(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleByURL failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article by url"))
	}

	return connect.NewResponse(&datahubv1.GetArticleByURLResponse{
		Article: articleContentToProto(article),
	}), nil
}

func (h *Handler) BatchGetArticlesByURLs(ctx context.Context, req *connect.Request[datahubv1.BatchGetArticlesByURLsRequest]) (*connect.Response[datahubv1.BatchGetArticlesByURLsResponse], error) {
	urls := req.Msg.GetUrls()
	if len(urls) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetArticlesByURLsResponse{
			Articles: map[string]*datahubv1.ArticleContent{},
		}), nil
	}
	if len(urls) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("urls exceeds the %d entry limit", maxLimit))
	}

	userID, err := optionalUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	found, err := h.articleRead.BatchGetByURLs(ctx, urls, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "BatchGetArticlesByURLs failed", "error", err, "count", len(urls))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get articles by urls"))
	}

	out := make(map[string]*datahubv1.ArticleContent, len(found))
	for url, article := range found {
		// A URL with no archived article is absent rather than mapped to an
		// empty message: the caller distinguishes "not fetched yet" from
		// "fetched and empty" and acts differently on each.
		if msg := articleContentToProto(article); msg != nil {
			out[url] = msg
		}
	}
	return connect.NewResponse(&datahubv1.BatchGetArticlesByURLsResponse{Articles: out}), nil
}

func (h *Handler) GetArticleContentByID(ctx context.Context, req *connect.Request[datahubv1.GetArticleContentByIDRequest]) (*connect.Response[datahubv1.GetArticleContentByIDResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	article, err := h.articleRead.GetContentByID(ctx, req.Msg.GetArticleId())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleContentByID failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article content"))
	}

	return connect.NewResponse(&datahubv1.GetArticleContentByIDResponse{
		Article: articleContentToProto(article),
	}), nil
}

func (h *Handler) ListArticlesCursor(ctx context.Context, req *connect.Request[datahubv1.ListArticlesCursorRequest]) (*connect.Response[datahubv1.ListArticlesCursorResponse], error) {
	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	articles, err := h.articleRead.ListCursor(ctx, userID, timePtrOrNil(req.Msg.GetCursor()), clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticlesCursor failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles"))
	}

	out := make([]*datahubv1.UserArticle, 0, len(articles))
	for _, a := range articles {
		out = append(out, userArticleToProto(a))
	}
	return connect.NewResponse(&datahubv1.ListArticlesCursorResponse{Articles: out}), nil
}

func (h *Handler) ListArticleIDsCursor(ctx context.Context, req *connect.Request[datahubv1.ListArticleIDsCursorRequest]) (*connect.Response[datahubv1.ListArticleIDsCursorResponse], error) {
	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	ids, err := h.articleRead.ListIDsCursor(ctx, userID, timePtrOrNil(req.Msg.GetCursor()), clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticleIDsCursor failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list article ids"))
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return connect.NewResponse(&datahubv1.ListArticleIDsCursorResponse{ArticleIds: out}), nil
}

func (h *Handler) BatchGetArticlesByIDs(ctx context.Context, req *connect.Request[datahubv1.BatchGetArticlesByIDsRequest]) (*connect.Response[datahubv1.BatchGetArticlesByIDsResponse], error) {
	raw := req.Msg.GetArticleIds()
	if len(raw) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetArticlesByIDsResponse{}), nil
	}
	if len(raw) > maxLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_ids exceeds the %d id limit", maxLimit))
	}

	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			// Skipping the unparseable one would return a short list the
			// caller renders positionally, silently dropping an article.
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("article_ids contains a value that is not a uuid: %w", err))
		}
		ids = append(ids, id)
	}

	articles, err := h.articleRead.BatchGetByIDs(ctx, ids)
	if err != nil {
		h.logger.ErrorContext(ctx, "BatchGetArticlesByIDs failed", "error", err, "count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get articles by ids"))
	}

	out := make([]*datahubv1.UserArticle, 0, len(articles))
	for _, a := range articles {
		out = append(out, userArticleToProto(a))
	}
	return connect.NewResponse(&datahubv1.BatchGetArticlesByIDsResponse{Articles: out}), nil
}

func (h *Handler) GetLatestArticleByFeedID(ctx context.Context, req *connect.Request[datahubv1.GetLatestArticleByFeedIDRequest]) (*connect.Response[datahubv1.GetLatestArticleByFeedIDResponse], error) {
	feedID, err := requiredUUID(req.Msg.GetFeedId(), "feed_id")
	if err != nil {
		return nil, err
	}

	article, err := h.articleRead.GetLatestByFeedID(ctx, feedID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetLatestArticleByFeedID failed", "error", err, "feed_id", feedID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get latest article"))
	}

	return connect.NewResponse(&datahubv1.GetLatestArticleByFeedIDResponse{
		Article: articleContentToProto(article),
	}), nil
}

// LookupArticleURL answers "" for an article that does not exist within the
// user's tenant, which is the same answer it gives for one that does not exist
// at all. NotFound would tell the caller which of the two it was, and that is
// a cross-tenant existence oracle.
func (h *Handler) LookupArticleURL(ctx context.Context, req *connect.Request[datahubv1.LookupArticleURLRequest]) (*connect.Response[datahubv1.LookupArticleURLResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	url, err := h.articleRead.LookupURL(ctx, req.Msg.GetArticleId(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "LookupArticleURL failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to look up article url"))
	}
	return connect.NewResponse(&datahubv1.LookupArticleURLResponse{Url: url}), nil
}

// ---------------------------------------------------------------------------
// §2.N Knowledge backfill reads
// ---------------------------------------------------------------------------

func (h *Handler) CountBackfillArticles(ctx context.Context, _ *connect.Request[datahubv1.CountBackfillArticlesRequest]) (*connect.Response[datahubv1.CountBackfillArticlesResponse], error) {
	count, err := h.knowledgeBackfill.CountArticles(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "CountBackfillArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count backfill articles"))
	}
	return connect.NewResponse(&datahubv1.CountBackfillArticlesResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) ListBackfillArticles(ctx context.Context, req *connect.Request[datahubv1.ListBackfillArticlesRequest]) (*connect.Response[datahubv1.ListBackfillArticlesResponse], error) {
	lastCreatedAt, lastArticleID, err := keysetCursor(req.Msg.GetLastCreatedAt(), req.Msg.GetLastArticleId(), "last_created_at", "last_article_id")
	if err != nil {
		return nil, err
	}

	articles, err := h.knowledgeBackfill.ListArticles(ctx, lastCreatedAt, lastArticleID, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListBackfillArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list backfill articles"))
	}

	out := make([]*datahubv1.BackfillArticle, 0, len(articles))
	for _, a := range articles {
		out = append(out, &datahubv1.BackfillArticle{
			ArticleId:   a.ArticleID.String(),
			UserId:      a.UserID.String(),
			CreatedAt:   timestampOrNil(a.CreatedAt),
			PublishedAt: timestampOrNil(a.PublishedAt),
			Title:       a.Title,
			Url:         a.URL,
		})
	}
	return connect.NewResponse(&datahubv1.ListBackfillArticlesResponse{Articles: out}), nil
}

func (h *Handler) CountBackfillSummaryTitles(ctx context.Context, _ *connect.Request[datahubv1.CountBackfillSummaryTitlesRequest]) (*connect.Response[datahubv1.CountBackfillSummaryTitlesResponse], error) {
	count, err := h.knowledgeBackfill.CountSummaryTitles(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "CountBackfillSummaryTitles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count backfill summary titles"))
	}
	return connect.NewResponse(&datahubv1.CountBackfillSummaryTitlesResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) ListBackfillSummaryTitles(ctx context.Context, req *connect.Request[datahubv1.ListBackfillSummaryTitlesRequest]) (*connect.Response[datahubv1.ListBackfillSummaryTitlesResponse], error) {
	lastGeneratedAt, lastVersionID, err := keysetCursor(req.Msg.GetLastGeneratedAt(), req.Msg.GetLastSummaryVersionId(), "last_generated_at", "last_summary_version_id")
	if err != nil {
		return nil, err
	}

	entries, err := h.knowledgeBackfill.ListSummaryTitles(ctx, lastGeneratedAt, lastVersionID, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListBackfillSummaryTitles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list backfill summary titles"))
	}

	out := make([]*datahubv1.BackfillSummaryTitle, 0, len(entries))
	for _, e := range entries {
		out = append(out, &datahubv1.BackfillSummaryTitle{
			SummaryVersionId: e.SummaryVersionID.String(),
			ArticleId:        e.ArticleID.String(),
			UserId:           e.UserID.String(),
			TenantId:         e.TenantID.String(),
			Title:            e.Title,
			GeneratedAt:      timestampOrNil(e.GeneratedAt),
		})
	}
	return connect.NewResponse(&datahubv1.ListBackfillSummaryTitlesResponse{Entries: out}), nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// requiredUUID rejects both an absent and an unparseable identifier at the
// delivery layer, so the caller gets InvalidArgument rather than an Internal
// wrapping whatever the query did with a zero UUID.
func requiredUUID(raw, field string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s is required", field))
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", field, err))
	}
	return id, nil
}

// optionalUUID maps an empty field to nil — "no scope", a different query —
// while still refusing a value that was sent and does not parse. Silently
// treating a malformed user id as "unscoped" would widen a tenant-scoped read
// into a global one.
func optionalUUID(raw, field string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", field, err))
	}
	return &id, nil
}

// keysetCursor enforces the all-or-nothing rule both backfill walks share.
//
// Accepting half a cursor would restart the walk from the beginning, and the
// jobs that drive these would re-emit every knowledge event they had already
// emitted — a silent duplicate replay rather than a visible failure.
func keysetCursor(ts *timestamppb.Timestamp, rawID, tsField, idField string) (*time.Time, *uuid.UUID, error) {
	hasTS := ts != nil && ts.IsValid()
	hasID := rawID != ""

	switch {
	case !hasTS && !hasID:
		return nil, nil, nil
	case hasTS != hasID:
		return nil, nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%s and %s must be sent together: half a keyset cursor would restart the walk from the beginning", tsField, idField))
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a uuid: %w", idField, err))
	}
	t := ts.AsTime()
	return &t, &id, nil
}

// timePtrOrNil keeps an unset cursor nil rather than turning it into the zero
// time, which the first-page query branches on.
func timePtrOrNil(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil || !ts.IsValid() {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func articleContentToProto(a *domain.ArticleContent) *datahubv1.ArticleContent {
	if a == nil {
		return nil
	}
	return &datahubv1.ArticleContent{
		Id:      a.ID,
		Title:   a.Title,
		Content: a.Content,
		Url:     a.URL,
		FeedId:  a.FeedID,
	}
}

func userArticleToProto(a *domain.Article) *datahubv1.UserArticle {
	if a == nil {
		return nil
	}
	// The zero feed id is sent as an empty string rather than
	// "00000000-...": an article whose feed was never resolved has no feed,
	// and a caller comparing against the nil UUID text would be comparing
	// against a value that means nothing to it.
	feedID := ""
	if a.FeedID != uuid.Nil {
		feedID = a.FeedID.String()
	}
	return &datahubv1.UserArticle{
		Id:          a.ID.String(),
		FeedId:      feedID,
		Title:       a.Title,
		Content:     a.Content,
		Url:         a.URL,
		Tags:        a.Tags,
		PublishedAt: timestampOrNil(a.PublishedAt),
		CreatedAt:   timestampOrNil(a.CreatedAt),
	}
}
