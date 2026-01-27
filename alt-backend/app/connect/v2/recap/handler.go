// Package recap implements the RecapService Connect-RPC handlers.
package recap

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	recapv2 "alt/gen/proto/alt/recap/v2"
	"alt/gen/proto/alt/recap/v2/recapv2connect"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	recapinternal "alt/internal/recap"
	recap_usecase "alt/usecase/recap_usecase"
)

// Handler implements the RecapService Connect-RPC service.
type Handler struct {
	recapUsecase       *recap_usecase.RecapUsecase
	clusterDraftLoader *recapinternal.ClusterDraftLoader
	logger             *slog.Logger
}

// NewHandler creates a new Recap service handler.
func NewHandler(
	recapUsecase *recap_usecase.RecapUsecase,
	clusterDraftLoader *recapinternal.ClusterDraftLoader,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		recapUsecase:       recapUsecase,
		clusterDraftLoader: clusterDraftLoader,
		logger:             logger,
	}
}

// Verify interface implementation at compile time.
var _ recapv2connect.RecapServiceHandler = (*Handler)(nil)

// GetSevenDayRecap returns 7-day recap summary (authentication required).
// Replaces GET /api/v1/recap/7days
func (h *Handler) GetSevenDayRecap(
	ctx context.Context,
	req *connect.Request[recapv2.GetSevenDayRecapRequest],
) (*connect.Response[recapv2.GetSevenDayRecapResponse], error) {
	// Authentication check
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	h.logger.InfoContext(ctx, "GetSevenDayRecap called", "user_id", userCtx.UserID)

	// Call usecase
	recap, err := h.recapUsecase.GetSevenDayRecap(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrRecapNotFound) {
			return nil, errorhandler.HandleNotFoundError(ctx, h.logger, "No 7-day recap available yet", "GetSevenDayRecap")
		}
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetSevenDayRecap")
	}

	// Convert domain to proto
	resp := domainToProto(recap)

	// Attach cluster draft if requested
	if req.Msg.GenreDraftId != nil && *req.Msg.GenreDraftId != "" && h.clusterDraftLoader != nil {
		draft, err := h.clusterDraftLoader.LoadDraft(*req.Msg.GenreDraftId)
		if err != nil {
			h.logger.WarnContext(ctx, "cluster draft loader failed", "error", err, "draft_id", *req.Msg.GenreDraftId)
		} else if draft != nil {
			resp.ClusterDraft = clusterDraftToProto(draft)
		}
	}

	return connect.NewResponse(resp), nil
}

// domainToProto converts domain.RecapSummary to proto response.
func domainToProto(recap *domain.RecapSummary) *recapv2.GetSevenDayRecapResponse {
	genres := make([]*recapv2.RecapGenre, len(recap.Genres))
	for i, g := range recap.Genres {
		genres[i] = &recapv2.RecapGenre{
			Genre:         g.Genre,
			Summary:       g.Summary,
			TopTerms:      g.TopTerms,
			ArticleCount:  int32(g.ArticleCount),
			ClusterCount:  int32(g.ClusterCount),
			EvidenceLinks: evidenceLinksToProto(g.EvidenceLinks),
			Bullets:       g.Bullets,
			References:    referencesToProto(g.References),
		}
	}

	return &recapv2.GetSevenDayRecapResponse{
		JobId:         recap.JobID,
		ExecutedAt:    recap.ExecutedAt.Format(time.RFC3339),
		WindowStart:   recap.WindowStart.Format(time.RFC3339),
		WindowEnd:     recap.WindowEnd.Format(time.RFC3339),
		TotalArticles: int32(recap.TotalArticles),
		Genres:        genres,
	}
}

// evidenceLinksToProto converts domain evidence links to proto.
func evidenceLinksToProto(links []domain.EvidenceLink) []*recapv2.EvidenceLink {
	result := make([]*recapv2.EvidenceLink, len(links))
	for i, l := range links {
		result[i] = &recapv2.EvidenceLink{
			ArticleId:   l.ArticleID,
			Title:       l.Title,
			SourceUrl:   l.SourceURL,
			PublishedAt: l.PublishedAt,
			Lang:        l.Lang,
		}
	}
	return result
}

// referencesToProto converts domain references to proto.
func referencesToProto(refs []domain.Reference) []*recapv2.Reference {
	result := make([]*recapv2.Reference, len(refs))
	for i, r := range refs {
		result[i] = &recapv2.Reference{
			Id:     int32(r.ID),
			Url:    r.URL,
			Domain: r.Domain,
		}
		if r.ArticleID != nil {
			result[i].ArticleId = r.ArticleID
		}
	}
	return result
}

// clusterDraftToProto converts domain ClusterDraft to proto.
func clusterDraftToProto(draft *domain.ClusterDraft) *recapv2.ClusterDraft {
	genres := make([]*recapv2.ClusterGenre, len(draft.Genres))
	for i, g := range draft.Genres {
		genres[i] = &recapv2.ClusterGenre{
			Genre:        g.Genre,
			SampleSize:   int32(g.SampleSize),
			ClusterCount: int32(g.ClusterCount),
			Clusters:     clusterSegmentsToProto(g.Clusters),
		}
	}

	return &recapv2.ClusterDraft{
		DraftId:      draft.ID,
		Description:  draft.Description,
		Source:       draft.Source,
		GeneratedAt:  draft.GeneratedAt.Format(time.RFC3339),
		TotalEntries: int32(draft.TotalEntries),
		Genres:       genres,
	}
}

// clusterSegmentsToProto converts domain ClusterSegments to proto.
func clusterSegmentsToProto(segments []domain.ClusterSegment) []*recapv2.ClusterSegment {
	result := make([]*recapv2.ClusterSegment, len(segments))
	for i, s := range segments {
		result[i] = &recapv2.ClusterSegment{
			ClusterId:                s.ClusterID,
			Label:                    s.Label,
			Count:                    int32(s.Count),
			MarginMean:               s.MarginMean,
			MarginStd:                s.MarginStd,
			TopBoostMean:             s.TopBoostMean,
			GraphBoostAvailableRatio: s.GraphBoostAvailableRatio,
			TagCountMean:             s.TagCountMean,
			TagEntropyMean:           s.TagEntropyMean,
			TopTags:                  s.TopTags,
			RepresentativeArticles:   clusterArticlesToProto(s.RepresentativeArticles),
		}
	}
	return result
}

// clusterArticlesToProto converts domain ClusterArticles to proto.
func clusterArticlesToProto(articles []domain.ClusterArticle) []*recapv2.ClusterArticle {
	result := make([]*recapv2.ClusterArticle, len(articles))
	for i, a := range articles {
		result[i] = &recapv2.ClusterArticle{
			ArticleId:      a.ArticleID,
			Margin:         a.Margin,
			TopBoost:       a.TopBoost,
			Strategy:       a.Strategy,
			TagCount:       int32(a.TagCount),
			CandidateCount: int32(a.CandidateCount),
			TopTags:        a.TopTags,
		}
	}
	return result
}
