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
	searchByUserUsecase *usecase.SearchByUserUsecase
}

// NewHandler creates a new search handler.
func NewHandler(searchByUserUsecase *usecase.SearchByUserUsecase) *Handler {
	return &Handler{searchByUserUsecase: searchByUserUsecase}
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
		logger.Logger.Error("search failed", "err", err, "user_id", userID, "offset", offset, "limit", limit)
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

	logger.Logger.Info("search ok", "query", query, "user_id", userID, "count", len(hits), "estimated_total", result.EstimatedTotalHits)

	return connect.NewResponse(&searchv2.SearchArticlesResponse{
		Query:              query,
		Hits:               hits,
		EstimatedTotalHits: result.EstimatedTotalHits,
	}), nil
}
