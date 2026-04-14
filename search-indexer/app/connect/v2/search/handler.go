// Package search provides Connect-RPC handlers for search operations.
package search

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	searchv2 "search-indexer/gen/proto/services/search/v2"
	"search-indexer/gen/proto/services/search/v2/searchv2connect"
	"search-indexer/logger"
	"search-indexer/usecase"
)

// Handler implements the SearchService Connect-RPC handler.
type Handler struct {
	searchByUserUsecase  *usecase.SearchByUserUsecase
	searchRecapsUsecase  *usecase.SearchRecapsUsecase
}

// NewHandler creates a new search handler.
func NewHandler(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase) *Handler {
	return &Handler{
		searchByUserUsecase: searchByUserUsecase,
		searchRecapsUsecase: searchRecapsUsecase,
	}
}

// Compile-time check that Handler implements SearchServiceHandler.
var _ searchv2connect.SearchServiceHandler = (*Handler)(nil)

// SearchArticles searches for articles matching the query.
func (h *Handler) SearchArticles(
	ctx context.Context,
	req *connect.Request[searchv2.SearchArticlesRequest],
) (*connect.Response[searchv2.SearchArticlesResponse], error) {
	query := req.Msg.Query
	userID := req.Msg.UserId
	offset := int64(req.Msg.Offset)
	limit := int64(req.Msg.Limit)

	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	result, err := h.searchByUserUsecase.ExecuteWithPagination(ctx, query, userID, offset, limit)
	if err != nil {
		logger.Logger.Error("search failed", "err", err, "user_id", userID, "query_hash", logger.HashQuery(query), "offset", offset, "limit", limit)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("search failed"))
	}

	hits := make([]*searchv2.SearchHit, 0, len(result.Hits))
	for _, doc := range result.Hits {
		tags := doc.Tags
		if tags == nil {
			tags = []string{}
		}
		hits = append(hits, &searchv2.SearchHit{
			Id:      doc.ID,
			Title:   doc.Title,
			Content: doc.Content,
			Tags:    tags,
		})
	}

	logger.Logger.Info("search ok", "query_hash", logger.HashQuery(query), "user_id", userID, "count", len(hits), "estimated_total", result.EstimatedTotalHits)

	return connect.NewResponse(&searchv2.SearchArticlesResponse{
		Query:              query,
		Hits:               hits,
		EstimatedTotalHits: result.EstimatedTotalHits,
	}), nil
}

// SearchRecaps searches recap genres by tag name via Meilisearch.
func (h *Handler) SearchRecaps(
	ctx context.Context,
	req *connect.Request[searchv2.SearchRecapsRequest],
) (*connect.Response[searchv2.SearchRecapsResponse], error) {
	if h.searchRecapsUsecase == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("recap search not configured"))
	}

	tagName := req.Msg.TagName
	query := req.Msg.GetQuery()
	limit := int(req.Msg.Limit)

	var result *usecase.SearchRecapsResult
	var err error

	switch {
	case query != "":
		result, err = h.searchRecapsUsecase.ExecuteByQuery(ctx, query, limit)
	case tagName != "":
		result, err = h.searchRecapsUsecase.Execute(ctx, tagName, limit)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query or tag_name is required"))
	}

	if err != nil {
		logger.Logger.Error("recap search failed", "err", err, "tag_name", tagName, "query_hash", logger.HashQuery(query))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("recap search failed"))
	}

	hits := make([]*searchv2.RecapSearchHit, 0, len(result.Hits))
	for _, doc := range result.Hits {
		topTerms := doc.TopTerms
		if topTerms == nil {
			topTerms = []string{}
		}
		bullets := doc.Bullets
		if bullets == nil {
			bullets = []string{}
		}
		tags := doc.Tags
		if tags == nil {
			tags = []string{}
		}
		hits = append(hits, &searchv2.RecapSearchHit{
			Id:         doc.ID,
			JobId:      doc.JobID,
			ExecutedAt: doc.ExecutedAt,
			WindowDays: int32(doc.WindowDays),
			Genre:      doc.Genre,
			Summary:    doc.Summary,
			TopTerms:   topTerms,
			Bullets:    bullets,
			Tags:       tags,
		})
	}

	logger.Logger.Info("recap search ok", "tag_name", tagName, "query_hash", logger.HashQuery(query), "count", len(hits), "estimated_total", result.EstimatedTotalHits)

	return connect.NewResponse(&searchv2.SearchRecapsResponse{
		Hits:               hits,
		EstimatedTotalHits: result.EstimatedTotalHits,
	}), nil
}
