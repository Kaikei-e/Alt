package handler

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/driver/sovereign_db"
)

func (h *SovereignHandler) GetKnowledgeHomeItems(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetKnowledgeHomeItemsRequest],
) (*connect.Response[sovereignv1.GetKnowledgeHomeItemsResponse], error) {
	msg := req.Msg
	userID := parseUUID(msg.UserId)

	var filter *sovereign_db.LensFilter
	if msg.Filter != nil {
		filter = &sovereign_db.LensFilter{
			QueryText:    msg.Filter.QueryText,
			TagNames:     msg.Filter.TagIds,
			SourceIDs:    msg.Filter.SourceIds,
			TimeWindow:   msg.Filter.TimeWindow,
			IncludeRecap: msg.Filter.IncludeRecap,
			IncludePulse: msg.Filter.IncludePulse,
			SortMode:     msg.Filter.SortMode,
		}
	}

	items, nextCursor, hasMore, err := h.readDB.GetKnowledgeHomeItems(ctx, userID, msg.Cursor, int(msg.Limit), filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetKnowledgeHomeItems: %w", err))
	}

	pbItems := make([]*sovereignv1.KnowledgeHomeItem, len(items))
	for i, item := range items {
		pbItems[i] = homeItemToProto(item)
	}

	return connect.NewResponse(&sovereignv1.GetKnowledgeHomeItemsResponse{
		Items:      pbItems,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

func (h *SovereignHandler) GetTodayDigest(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetTodayDigestRequest],
) (*connect.Response[sovereignv1.GetTodayDigestResponse], error) {
	userID := parseUUID(req.Msg.UserId)
	date, _ := time.Parse("2006-01-02", req.Msg.Date)
	if date.IsZero() {
		date = time.Now()
	}

	digest, err := h.readDB.GetTodayDigest(ctx, userID, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetTodayDigest: %w", err))
	}

	var pb *sovereignv1.TodayDigest
	if digest != nil {
		pb = &sovereignv1.TodayDigest{
			UserId:                digest.UserID.String(),
			DigestDate:            digest.DigestDate.Format("2006-01-02"),
			NewArticles:           int32(digest.NewArticles),
			SummarizedArticles:    int32(digest.SummarizedArticles),
			UnsummarizedArticles:  int32(digest.UnsummarizedArticles),
			TopTags:               digest.TopTags,
			UpdatedAt:             timestamppb.New(digest.UpdatedAt),
			WeeklyRecapAvailable:  digest.WeeklyRecapAvailable,
			EveningPulseAvailable: digest.EveningPulseAvailable,
		}
	}

	return connect.NewResponse(&sovereignv1.GetTodayDigestResponse{Digest: pb}), nil
}

func (h *SovereignHandler) GetRecallCandidates(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetRecallCandidatesRequest],
) (*connect.Response[sovereignv1.GetRecallCandidatesResponse], error) {
	userID := parseUUID(req.Msg.UserId)
	candidates, err := h.readDB.GetRecallCandidates(ctx, userID, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetRecallCandidates: %w", err))
	}

	pbCandidates := make([]*sovereignv1.RecallCandidate, len(candidates))
	for i, c := range candidates {
		pb := &sovereignv1.RecallCandidate{
			UserId:            c.UserID.String(),
			ItemKey:           c.ItemKey,
			RecallScore:       c.RecallScore,
			UpdatedAt:         timestamppb.New(c.UpdatedAt),
			ProjectionVersion: int32(c.ProjectionVersion),
		}
		if c.NextSuggestAt != nil {
			pb.NextSuggestAt = timestamppb.New(*c.NextSuggestAt)
		}
		if c.FirstEligibleAt != nil {
			pb.FirstEligibleAt = timestamppb.New(*c.FirstEligibleAt)
		}
		for _, r := range c.Reasons {
			pb.Reasons = append(pb.Reasons, &sovereignv1.RecallReason{
				Type: r.Type, Description: r.Description, SourceItemKey: r.SourceItemKey,
			})
		}
		if c.Item != nil {
			pb.Item = homeItemToProto(*c.Item)
		}
		pbCandidates[i] = pb
	}

	return connect.NewResponse(&sovereignv1.GetRecallCandidatesResponse{Candidates: pbCandidates}), nil
}

func (h *SovereignHandler) ListDistinctUserIDs(
	ctx context.Context,
	_ *connect.Request[sovereignv1.ListDistinctUserIDsRequest],
) (*connect.Response[sovereignv1.ListDistinctUserIDsResponse], error) {
	ids, err := h.readDB.ListDistinctUserIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListDistinctUserIDs: %w", err))
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	return connect.NewResponse(&sovereignv1.ListDistinctUserIDsResponse{UserIds: strs}), nil
}

func (h *SovereignHandler) CountNeedToKnowItems(
	ctx context.Context,
	req *connect.Request[sovereignv1.CountNeedToKnowItemsRequest],
) (*connect.Response[sovereignv1.CountNeedToKnowItemsResponse], error) {
	userID := parseUUID(req.Msg.UserId)
	date, _ := time.Parse("2006-01-02", req.Msg.Date)
	if date.IsZero() {
		date = time.Now()
	}
	count, err := h.readDB.CountNeedToKnowItems(ctx, userID, date)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CountNeedToKnowItems: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CountNeedToKnowItemsResponse{Count: int32(count)}), nil
}

// --- helper: convert domain home item to proto ---

func homeItemToProto(item sovereign_db.KnowledgeHomeItem) *sovereignv1.KnowledgeHomeItem {
	pb := &sovereignv1.KnowledgeHomeItem{
		UserId:            item.UserID.String(),
		TenantId:          item.TenantID.String(),
		ItemKey:           item.ItemKey,
		ItemType:          item.ItemType,
		Title:             item.Title,
		SummaryExcerpt:    item.SummaryExcerpt,
		Tags:              item.Tags,
		Score:             item.Score,
		GeneratedAt:       timestamppb.New(item.GeneratedAt),
		UpdatedAt:         timestamppb.New(item.UpdatedAt),
		ProjectionVersion: int32(item.ProjectionVersion),
		SummaryState:      item.SummaryState,
		SupersedeState:    item.SupersedeState,
		PreviousRefJson:   item.PreviousRefJSON,
		Link:              item.Link,
	}
	if item.PrimaryRefID != nil {
		pb.PrimaryRefId = item.PrimaryRefID.String()
	}
	if item.FreshnessAt != nil {
		pb.FreshnessAt = timestamppb.New(*item.FreshnessAt)
	}
	if item.PublishedAt != nil {
		pb.PublishedAt = timestamppb.New(*item.PublishedAt)
	}
	if item.LastInteractedAt != nil {
		pb.LastInteractedAt = timestamppb.New(*item.LastInteractedAt)
	}
	if item.DismissedAt != nil {
		pb.DismissedAt = timestamppb.New(*item.DismissedAt)
	}
	if item.SupersededAt != nil {
		pb.SupersededAt = timestamppb.New(*item.SupersededAt)
	}
	for _, r := range item.WhyReasons {
		pb.WhyReasons = append(pb.WhyReasons, &sovereignv1.WhyReason{
			Code: r.Code, RefId: r.RefID, Tag: r.Tag,
		})
	}
	return pb
}
