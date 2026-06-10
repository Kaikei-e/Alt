// Package knowledge_trail provides the Connect-RPC handler for KnowledgeTrailService.
package knowledge_trail

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgetrailv1 "alt/gen/proto/alt/knowledge_trail/v1"
	"alt/usecase/get_knowledge_trail_usecase"
)

// Handler implements knowledgetrailv1connect.KnowledgeTrailServiceHandler.
type Handler struct {
	getTrailUsecase *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase
	logger          *slog.Logger
}

// NewHandler creates a new KnowledgeTrailService handler.
func NewHandler(getTrail *get_knowledge_trail_usecase.GetKnowledgeTrailUsecase, logger *slog.Logger) *Handler {
	return &Handler{getTrailUsecase: getTrail, logger: logger}
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
