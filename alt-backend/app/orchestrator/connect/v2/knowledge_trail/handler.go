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
	"alt/orchestrator/usecase/get_knowledge_trail_usecase"
	"alt/orchestrator/usecase/resolve_trail_branch_usecase"
)

// Handler implements knowledgetrailv1connect.KnowledgeTrailServiceHandler.
type Handler struct {
	getTrailUsecase *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase
	resolveUsecase  *resolve_trail_branch_usecase.ResolveTrailBranchUsecase
	logger          *slog.Logger
}

// NewHandler creates a new KnowledgeTrailService handler.
func NewHandler(
	getTrail *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase,
	resolve *resolve_trail_branch_usecase.ResolveTrailBranchUsecase,
	logger *slog.Logger,
) *Handler {
	return &Handler{getTrailUsecase: getTrail, resolveUsecase: resolve, logger: logger}
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
		footprints[i] = &knowledgetrailv1.Footprint{
			FootprintKey: fp.FootprintKey,
			Verb:         fp.Verb,
			ItemKey:      fp.ItemKey,
			Title:        fp.Title,
			Excerpt:      fp.Excerpt,
			Tags:         fp.Tags,
			Note:         fp.Note,
			OccurredAt:   fp.OccurredAt.UTC().Format(time.RFC3339),
			Wear:         fp.Wear,
		}
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
	}), nil
}
