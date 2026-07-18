// Package knowledge_trail provides the Connect-RPC handler for KnowledgeTrailService.
package knowledge_trail

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgetrailv1 "alt/gen/proto/alt/knowledge_trail/v1"
	"alt/orchestrator/usecase/emit_trail_outcome_usecase"
	"alt/orchestrator/usecase/get_knowledge_trail_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/resolve_trail_branch_usecase"
	"alt/orchestrator/usecase/search_trail_usecase"
)

// Handler implements knowledgetrailv1connect.KnowledgeTrailServiceHandler.
type Handler struct {
	getTrailUsecase *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase
	resolveUsecase  *resolve_trail_branch_usecase.ResolveTrailBranchUsecase
	emitUsecase     *emit_trail_outcome_usecase.EmitTrailOutcomeUsecase
	searchUsecase   *search_trail_usecase.SearchTrailUsecase
	imageProxy      *image_proxy_usecase.ImageProxyUsecase
	logger          *slog.Logger
}

// NewHandler creates a new KnowledgeTrailService handler. imageProxy signs
// episode thumbnails (D29); a nil imageProxy (the feature disabled, mirroring
// feeds.enrichWithProxyURLs) leaves thumbnails empty rather than leaking a
// raw, unsigned OG image URL to the wire.
func NewHandler(
	getTrail *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase,
	resolve *resolve_trail_branch_usecase.ResolveTrailBranchUsecase,
	emit *emit_trail_outcome_usecase.EmitTrailOutcomeUsecase,
	search *search_trail_usecase.SearchTrailUsecase,
	imageProxy *image_proxy_usecase.ImageProxyUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{getTrailUsecase: getTrail, resolveUsecase: resolve, emitUsecase: emit, searchUsecase: search, imageProxy: imageProxy, logger: logger}
}

// ResolveBranch records a user's take/dismiss of a proposed branch.
func (h *Handler) ResolveBranch(
	ctx context.Context,
	req *connect.Request[knowledgetrailv1.ResolveBranchRequest],
) (*connect.Response[knowledgetrailv1.ResolveBranchResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	msg := req.Msg
	if err := h.resolveUsecase.Execute(ctx, user.UserID, user.TenantID, msg.BranchKey, msg.Resolution, msg.ClientResolutionId); err != nil {
		if errors.Is(err, resolve_trail_branch_usecase.ErrInvalidRequest) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&knowledgetrailv1.ResolveBranchResponse{Ok: true}), nil
}

// EmitTrailOutcome records the raw dwell observed after a taken branch. Rule 8:
// an unwired usecase panics rather than silently swallowing outcomes.
func (h *Handler) EmitTrailOutcome(
	ctx context.Context,
	req *connect.Request[knowledgetrailv1.EmitTrailOutcomeRequest],
) (*connect.Response[knowledgetrailv1.EmitTrailOutcomeResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if h.emitUsecase == nil {
		panic("knowledge_trail.Handler: EmitTrailOutcome reached with unwired emit usecase (DI gap)")
	}
	msg := req.Msg
	if err := h.emitUsecase.Execute(ctx, user.UserID, user.TenantID, msg.BranchKey, msg.ItemKey, msg.DwellMs); err != nil {
		if errors.Is(err, emit_trail_outcome_usecase.ErrInvalidRequest) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&knowledgetrailv1.EmitTrailOutcomeResponse{Ok: true}), nil
}

// GetTrail returns the user's footprint spine, reverse-chronological.
func (h *Handler) GetTrail(
	ctx context.Context,
	req *connect.Request[knowledgetrailv1.GetTrailRequest],
) (*connect.Response[knowledgetrailv1.GetTrailResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	cursor := ""
	if req.Msg.Cursor != nil {
		cursor = *req.Msg.Cursor
	}

	result, err := h.getTrailUsecase.Execute(ctx, user.UserID, cursor, int(req.Msg.Limit), req.Msg.FilterTags)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	footprints := make([]*knowledgetrailv1.Footprint, len(result.Footprints))
	for i, fp := range result.Footprints {
		footprints[i] = mapFootprint(fp)
	}

	branches := make([]*knowledgetrailv1.Branch, len(result.Branches))
	for i, b := range result.Branches {
		refs := make([]*knowledgetrailv1.TrailEvidenceRef, len(b.EvidenceRefs))
		for j, r := range b.EvidenceRefs {
			refs[j] = &knowledgetrailv1.TrailEvidenceRef{RefId: r.RefID, Label: r.Label, Kind: r.Kind}
		}
		branches[i] = &knowledgetrailv1.Branch{
			BranchKey:     b.BranchKey,
			AnchorItemKey: b.AnchorItemKey,
			RelationKind:  b.RelationKind,
			Why:           b.Why,
			EvidenceRefs:  refs,
			Confidence:    b.Confidence,
			TargetItemKey: b.TargetItemKey,
			TargetTitle:   b.TargetTitle,
		}
	}

	return connect.NewResponse(&knowledgetrailv1.GetTrailResponse{
		Footprints: footprints,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		Branches:   branches,
		Episodes:   h.mapEpisodes(result.Episodes),
	}), nil
}

// SearchTrail performs full-text search over what the user actually read
// (Wave 9, D25) and returns the episodes containing a hit, anchored. Rule 8:
// an unwired usecase panics rather than silently returning an empty result —
// mirrors EmitTrailOutcome's guard.
func (h *Handler) SearchTrail(
	ctx context.Context,
	req *connect.Request[knowledgetrailv1.SearchTrailRequest],
) (*connect.Response[knowledgetrailv1.SearchTrailResponse], error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if h.searchUsecase == nil {
		panic("knowledge_trail.Handler: SearchTrail reached with unwired search usecase (DI gap)")
	}
	msg := req.Msg
	result, err := h.searchUsecase.Execute(ctx, user.UserID, msg.Query, int(msg.Limit))
	if err != nil {
		if errors.Is(err, search_trail_usecase.ErrInvalidRequest) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&knowledgetrailv1.SearchTrailResponse{
		Episodes:        h.mapEpisodes(result.Episodes),
		MatchedItemKeys: result.MatchedItemKeys,
	}), nil
}

// mapEpisodes maps domain episodes to their wire form, signing each
// episode's thumbnail (D29). Shared by GetTrail and SearchTrail so the two
// read paths can never diverge on episode/footprint mapping.
func (h *Handler) mapEpisodes(episodes []domain.TrailEpisode) []*knowledgetrailv1.Episode {
	out := make([]*knowledgetrailv1.Episode, len(episodes))
	for i, ep := range episodes {
		epFootprints := make([]*knowledgetrailv1.Footprint, len(ep.Footprints))
		for j, fp := range ep.Footprints {
			epFootprints[j] = mapFootprint(fp)
		}
		out[i] = &knowledgetrailv1.Episode{
			EpisodeKey:   ep.EpisodeKey,
			Wear:         ep.Wear,
			ThumbnailUrl: h.signThumbnail(ep.ThumbnailURL),
			Footprints:   epFootprints,
		}
	}
	return out
}

// signThumbnail signs a raw OG image URL through the image-proxy signer
// (D29), mirroring feeds.enrichWithProxyURLs. A raw URL never reaches the
// wire unsigned: with no image proxy wired, or no URL resolved, the card
// degrades to text.
func (h *Handler) signThumbnail(rawURL string) string {
	if rawURL == "" || h.imageProxy == nil {
		return ""
	}
	return h.imageProxy.GenerateProxyURL(rawURL)
}

// mapFootprint maps one domain footprint to its wire form.
func mapFootprint(fp domain.TrailFootprint) *knowledgetrailv1.Footprint {
	firstOccurredAt := fp.FirstOccurredAt
	if firstOccurredAt.IsZero() {
		firstOccurredAt = fp.OccurredAt
	}
	contactCount := max(fp.ContactCount, 1)
	return &knowledgetrailv1.Footprint{
		FootprintKey:    fp.FootprintKey,
		Verb:            fp.Verb,
		ItemKey:         fp.ItemKey,
		Title:           fp.Title,
		Excerpt:         fp.Excerpt,
		Tags:            fp.Tags,
		Note:            fp.Note,
		OccurredAt:      fp.OccurredAt.UTC().Format(time.RFC3339),
		Wear:            fp.Wear,
		ContactCount:    int32(contactCount), //nolint:gosec // >= 1, bounded upstream
		FirstOccurredAt: firstOccurredAt.UTC().Format(time.RFC3339),
	}
}
