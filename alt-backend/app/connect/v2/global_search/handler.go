// Package global_search provides Connect-RPC handlers for the GlobalSearchService.
package global_search

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	searchv2 "alt/gen/proto/alt/search/v2"
	"alt/gen/proto/alt/search/v2/searchv2connect"
	"alt/usecase/global_search_usecase"
)

// Handler implements the GlobalSearchService Connect-RPC handler.
type Handler struct {
	globalSearchUsecase *global_search_usecase.GlobalSearchUsecase
	logger              *slog.Logger
}

// Compile-time check that Handler implements GlobalSearchServiceHandler.
var _ searchv2connect.GlobalSearchServiceHandler = (*Handler)(nil)

// NewHandler creates a new GlobalSearchService handler.
func NewHandler(
	globalSearchUsecase *global_search_usecase.GlobalSearchUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		globalSearchUsecase: globalSearchUsecase,
		logger:              logger,
	}
}

// SearchEverything performs a federated search across all content verticals.
func (h *Handler) SearchEverything(
	ctx context.Context,
	req *connect.Request[searchv2.SearchEverythingRequest],
) (*connect.Response[searchv2.SearchEverythingResponse], error) {
	query := req.Msg.Query
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query is required"))
	}

	result, err := h.globalSearchUsecase.Execute(
		ctx,
		query,
		int(req.Msg.ArticleLimit),
		int(req.Msg.RecapLimit),
		int(req.Msg.TagLimit),
	)
	if err != nil {
		h.logger.ErrorContext(ctx, "global search failed", "error", err, "query", query)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("search failed"))
	}

	resp := &searchv2.SearchEverythingResponse{
		Query:            query,
		DegradedSections: result.DegradedSections,
		SearchedAt:       result.SearchedAt.Format(time.RFC3339),
	}

	if result.Articles != nil {
		resp.ArticleSection = toArticleSection(result.Articles)
	}
	if result.Recaps != nil {
		resp.RecapSection = toRecapSection(result.Recaps)
	}
	if result.Tags != nil {
		resp.TagSection = toTagSection(result.Tags)
	}

	h.logger.InfoContext(ctx, "global search ok", "query", query, "degraded", result.DegradedSections)

	return connect.NewResponse(resp), nil
}

func toArticleSection(s *domain.ArticleSearchSection) *searchv2.ArticleSection {
	hits := make([]*searchv2.GlobalArticleHit, len(s.Hits))
	for i, h := range s.Hits {
		tags := h.Tags
		if tags == nil {
			tags = []string{}
		}
		matched := h.MatchedFields
		if matched == nil {
			matched = []string{}
		}
		hits[i] = &searchv2.GlobalArticleHit{
			Id:            h.ID,
			Title:         h.Title,
			Snippet:       h.Snippet,
			Link:          h.Link,
			Tags:          tags,
			MatchedFields: matched,
		}
	}
	return &searchv2.ArticleSection{
		Hits:           hits,
		EstimatedTotal: s.EstimatedTotal,
		HasMore:        s.HasMore,
	}
}

func toRecapSection(s *domain.RecapSearchSection) *searchv2.RecapSection {
	hits := make([]*searchv2.GlobalRecapHit, len(s.Hits))
	for i, h := range s.Hits {
		topTerms := h.TopTerms
		if topTerms == nil {
			topTerms = []string{}
		}
		tags := h.Tags
		if tags == nil {
			tags = []string{}
		}
		hits[i] = &searchv2.GlobalRecapHit{
			Id:         h.ID,
			JobId:      h.JobID,
			Genre:      h.Genre,
			Summary:    h.Summary,
			TopTerms:   topTerms,
			Tags:       tags,
			WindowDays: int32(h.WindowDays),
			ExecutedAt: h.ExecutedAt,
		}
	}
	return &searchv2.RecapSection{
		Hits:           hits,
		EstimatedTotal: s.EstimatedTotal,
		HasMore:        s.HasMore,
	}
}

func toTagSection(s *domain.TagSearchSection) *searchv2.TagSection {
	hits := make([]*searchv2.GlobalTagHit, len(s.Hits))
	for i, h := range s.Hits {
		hits[i] = &searchv2.GlobalTagHit{
			TagName:      h.TagName,
			ArticleCount: int32(h.ArticleCount),
		}
	}
	return &searchv2.TagSection{
		Hits:  hits,
		Total: s.Total,
	}
}
