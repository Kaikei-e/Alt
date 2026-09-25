package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *Handler) ListArticlesWithTags(ctx context.Context, req *connect.Request[datahubv1.ListArticlesWithTagsRequest]) (*connect.Response[datahubv1.ListArticlesWithTagsResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.listArticles.ListArticlesWithTags(ctx, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListArticlesWithTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles"))
	}

	resp := &datahubv1.ListArticlesWithTagsResponse{
		Articles: toProtoArticles(articles),
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) ListArticlesWithTagsForward(ctx context.Context, req *connect.Request[datahubv1.ListArticlesWithTagsForwardRequest]) (*connect.Response[datahubv1.ListArticlesWithTagsForwardResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	incrementalMark := req.Msg.IncrementalMark.AsTime()

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.listArticlesForward.ListArticlesWithTagsForward(ctx, &incrementalMark, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListArticlesWithTagsForward failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles forward"))
	}

	resp := &datahubv1.ListArticlesWithTagsForwardResponse{
		Articles: toProtoArticles(articles),
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) ListDeletedArticles(ctx context.Context, req *connect.Request[datahubv1.ListDeletedArticlesRequest]) (*connect.Response[datahubv1.ListDeletedArticlesResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	var lastDeletedAt *time.Time
	if req.Msg.LastDeletedAt != nil {
		t := req.Msg.LastDeletedAt.AsTime()
		lastDeletedAt = &t
	}

	deletedArticles, nextDeletedAt, err := h.listDeleted.ListDeletedArticles(ctx, lastDeletedAt, limit)
	if err != nil {
		h.logger.Error("ListDeletedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list deleted articles"))
	}

	protoArticles := make([]*datahubv1.DeletedArticle, len(deletedArticles))
	for i, da := range deletedArticles {
		protoArticles[i] = &datahubv1.DeletedArticle{
			Id:        da.ID,
			DeletedAt: timestamppb.New(da.DeletedAt),
		}
	}

	resp := &datahubv1.ListDeletedArticlesResponse{
		Articles: protoArticles,
	}
	if nextDeletedAt != nil {
		resp.NextDeletedAt = timestamppb.New(*nextDeletedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) GetLatestArticleTimestamp(ctx context.Context, _ *connect.Request[datahubv1.GetLatestArticleTimestampRequest]) (*connect.Response[datahubv1.GetLatestArticleTimestampResponse], error) {
	ts, err := h.getLatestTimestamp.GetLatestArticleTimestamp(ctx)
	if err != nil {
		h.logger.Error("GetLatestArticleTimestamp failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get latest timestamp"))
	}

	resp := &datahubv1.GetLatestArticleTimestampResponse{}
	if ts != nil {
		resp.LatestCreatedAt = timestamppb.New(*ts)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) GetArticleByID(ctx context.Context, req *connect.Request[datahubv1.GetArticleByIDRequest]) (*connect.Response[datahubv1.GetArticleByIDResponse], error) {
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	article, err := h.getArticleByID.GetArticleByID(ctx, req.Msg.ArticleId)
	if err != nil {
		// search-indexer reads NotFound as "the row is gone", skips the
		// article and ACKs the message. A pool exhaustion or a transient DB
		// blip answered with the same code would drop that article from the
		// index for good, so only the driver's absence sentinel earns it.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
		}
		h.logger.Error("GetArticleByID failed", "article_id", req.Msg.ArticleId, "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article"))
	}

	return connect.NewResponse(&datahubv1.GetArticleByIDResponse{
		Article: toProtoArticle(article),
	}), nil
}

func (h *Handler) GetArticleContent(ctx context.Context, req *connect.Request[datahubv1.GetArticleContentRequest]) (*connect.Response[datahubv1.GetArticleContentResponse], error) {
	if h.getArticleContent == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	content, err := h.getArticleContent.GetArticleContent(ctx, req.Msg.ArticleId)
	if err != nil {
		h.logger.Error("GetArticleContent failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article content"))
	}
	if content == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
	}

	return connect.NewResponse(&datahubv1.GetArticleContentResponse{
		ArticleId: content.ID,
		Title:     content.Title,
		Content:   content.Content,
		Url:       content.URL,
		UserId:    content.UserID,
	}), nil
}

// ---------------------------------------------------------------------------
// §2.C Article reference for the recall rail
// ---------------------------------------------------------------------------

// GetArticleTitleAndLink answers a missing article with found=false and no
// error.
//
// NotFound would be the wrong Connect code here. The rail asks about
// candidates that knowledge-sovereign proposed, and an article deleted since
// then is an ordinary outcome the caller renders around — turning it into an
// error would put a routine miss in every error budget and every log.
func (h *Handler) GetArticleTitleAndLink(ctx context.Context, req *connect.Request[datahubv1.GetArticleTitleAndLinkRequest]) (*connect.Response[datahubv1.GetArticleTitleAndLinkResponse], error) {
	articleID := req.Msg.GetArticleId()
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	ref, err := h.articleRef.ArticleRef(ctx, articleID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleTitleAndLink failed", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article title and link"))
	}
	if ref == nil {
		return connect.NewResponse(&datahubv1.GetArticleTitleAndLinkResponse{Found: false}), nil
	}

	resp := &datahubv1.GetArticleTitleAndLinkResponse{
		Found: true,
		Title: ref.Title,
		Url:   ref.Link,
	}
	if ref.PublishedAt != nil {
		resp.PublishedAt = timestamppb.New(*ref.PublishedAt)
	}
	return connect.NewResponse(resp), nil
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

	source, err := h.articleRead.LookupSource(ctx, req.Msg.GetArticleId(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "LookupArticleURL failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to look up article url"))
	}
	return connect.NewResponse(&datahubv1.LookupArticleURLResponse{
		Url:   source.URL,
		Title: source.Title,
	}), nil
}
