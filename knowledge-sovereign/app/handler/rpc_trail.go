package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// GetTrailFootprints returns the user's footprint spine, reverse-chronological.
func (h *SovereignHandler) GetTrailFootprints(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetTrailFootprintsRequest],
) (*connect.Response[sovereignv1.GetTrailFootprintsResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	footprints, nextCursor, hasMore, err := h.readDB.GetTrailFootprints(ctx, userID, msg.Cursor, int(msg.Limit), msg.FilterTags)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetTrailFootprints: %w", err))
	}

	branches, err := h.readDB.GetOpenTrailBranches(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetOpenTrailBranches: %w", err))
	}

	pb := make([]*sovereignv1.TrailFootprint, len(footprints))
	for i, fp := range footprints {
		pb[i] = &sovereignv1.TrailFootprint{
			UserId:          fp.UserID.String(),
			TenantId:        fp.TenantID.String(),
			FootprintKey:    fp.FootprintKey,
			Verb:            fp.Verb,
			ItemKey:         fp.ItemKey,
			Title:           fp.Title,
			Excerpt:         fp.Excerpt,
			Tags:            fp.Tags,
			Note:            fp.Note,
			SourceEventType: fp.SourceEventType,
			OccurredAt:      timestamppb.New(fp.OccurredAt),
			Wear:            fp.Wear,
		}
	}

	pbBranches := make([]*sovereignv1.TrailBranch, len(branches))
	for i, b := range branches {
		refs := make([]*sovereignv1.TrailEvidenceRef, len(b.EvidenceRefs))
		for j, r := range b.EvidenceRefs {
			refs[j] = &sovereignv1.TrailEvidenceRef{RefId: r.RefID, Label: r.Label, Kind: r.Kind}
		}
		pbBranches[i] = &sovereignv1.TrailBranch{
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

	return connect.NewResponse(&sovereignv1.GetTrailFootprintsResponse{
		Footprints: pb,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Branches:   pbBranches,
	}), nil
}
