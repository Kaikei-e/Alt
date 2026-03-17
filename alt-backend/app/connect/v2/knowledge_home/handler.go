// Package knowledge_home provides the Connect-RPC handler for KnowledgeHomeService.
package knowledge_home

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/port/feature_flag_port"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
)

// Handler implements KnowledgeHomeServiceHandler.
type Handler struct {
	getHomeUsecase     *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	trackSeenUsecase   *track_home_seen_usecase.TrackHomeSeenUsecase
	trackActionUsecase *track_home_action_usecase.TrackHomeActionUsecase
	featureFlagPort    feature_flag_port.FeatureFlagPort
	logger             *slog.Logger
}

// Compile-time interface verification.
var _ knowledgehomev1connect.KnowledgeHomeServiceHandler = (*Handler)(nil)

// NewHandler creates a new KnowledgeHomeService handler.
func NewHandler(
	getHome *get_knowledge_home_usecase.GetKnowledgeHomeUsecase,
	trackSeen *track_home_seen_usecase.TrackHomeSeenUsecase,
	trackAction *track_home_action_usecase.TrackHomeActionUsecase,
	featureFlag feature_flag_port.FeatureFlagPort,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		getHomeUsecase:     getHome,
		trackSeenUsecase:   trackSeen,
		trackActionUsecase: trackAction,
		featureFlagPort:    featureFlag,
		logger:             logger,
	}
}

// GetKnowledgeHome returns the Knowledge Home feed.
func (h *Handler) GetKnowledgeHome(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetKnowledgeHomeRequest],
) (*connect.Response[knowledgehomev1.GetKnowledgeHomeResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Feature flag guard: deny access if Knowledge Home page is disabled for this user
	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagKnowledgeHomePage, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("knowledge home is not enabled for this user"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var cursor string
	if req.Msg.Cursor != nil {
		cursor = *req.Msg.Cursor
	}

	date := time.Now()
	if req.Msg.Date != nil && *req.Msg.Date != "" {
		parsed, err := time.Parse("2006-01-02", *req.Msg.Date)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err))
		}
		date = parsed
	}

	result, err := h.getHomeUsecase.Execute(ctx, user.UserID, cursor, limit, date)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetKnowledgeHome")
	}

	// Convert to proto
	protoItems := make([]*knowledgehomev1.KnowledgeHomeItem, 0, len(result.Items))
	for _, item := range result.Items {
		protoItem := &knowledgehomev1.KnowledgeHomeItem{
			ItemKey:  item.ItemKey,
			ItemType: item.ItemType,
			Title:    item.Title,
			Tags:     item.Tags,
			Score:    item.Score,
		}

		if item.PrimaryRefID != nil {
			refID := item.PrimaryRefID.String()
			if item.ItemType == "article" {
				protoItem.ArticleId = &refID
			} else if item.ItemType == "recap_anchor" {
				protoItem.RecapId = &refID
			}
		}

		if item.PublishedAt != nil {
			protoItem.PublishedAt = item.PublishedAt.Format(time.RFC3339)
		}

		if item.SummaryExcerpt != "" {
			excerpt := item.SummaryExcerpt
			protoItem.SummaryExcerpt = &excerpt
		}

		protoWhys := make([]*knowledgehomev1.WhyReason, 0, len(item.WhyReasons))
		for _, why := range item.WhyReasons {
			protoWhy := &knowledgehomev1.WhyReason{
				Code: why.Code,
			}
			if why.RefID != "" {
				protoWhy.RefId = &why.RefID
			}
			if why.Tag != "" {
				protoWhy.Tag = &why.Tag
			}
			protoWhys = append(protoWhys, protoWhy)
		}
		protoItem.Why = protoWhys

		protoItems = append(protoItems, protoItem)
	}

	digest := &knowledgehomev1.TodayDigest{
		Date:                  result.Digest.DigestDate.Format("2006-01-02"),
		NewArticles:           int32(result.Digest.NewArticles),
		SummarizedArticles:    int32(result.Digest.SummarizedArticles),
		UnsummarizedArticles:  int32(result.Digest.UnsummarizedArticles),
		TopTags:               result.Digest.TopTags,
		WeeklyRecapAvailable:  result.Digest.WeeklyRecapAvailable,
		EveningPulseAvailable: result.Digest.EveningPulseAvailable,
	}

	// Build feature flag statuses for the response
	var featureFlags []*knowledgehomev1.FeatureFlagStatus
	if h.featureFlagPort != nil {
		flags := []string{
			domain.FlagKnowledgeHomePage,
			domain.FlagKnowledgeHomeTracking,
			domain.FlagKnowledgeHomeProjectionV2,
		}
		for _, flag := range flags {
			featureFlags = append(featureFlags, &knowledgehomev1.FeatureFlagStatus{
				Name:    flag,
				Enabled: h.featureFlagPort.IsEnabled(flag, user.UserID),
			})
		}
	}

	return connect.NewResponse(&knowledgehomev1.GetKnowledgeHomeResponse{
		TodayDigest:  digest,
		Items:        protoItems,
		NextCursor:   result.NextCursor,
		HasMore:      result.HasMore,
		DegradedMode: result.Degraded,
		GeneratedAt:  result.GeneratedAt.Format(time.RFC3339),
		FeatureFlags: featureFlags,
	}), nil
}

// TrackHomeItemsSeen records which items were visible on screen.
func (h *Handler) TrackHomeItemsSeen(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackHomeItemsSeenRequest],
) (*connect.Response[knowledgehomev1.TrackHomeItemsSeenResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if len(req.Msg.ItemKeys) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item_keys is required"))
	}

	if err := h.trackSeenUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKeys, req.Msg.ExposureSessionId); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeItemsSeen")
	}

	return connect.NewResponse(&knowledgehomev1.TrackHomeItemsSeenResponse{}), nil
}

// TrackHomeAction records a user action on a home item.
func (h *Handler) TrackHomeAction(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackHomeActionRequest],
) (*connect.Response[knowledgehomev1.TrackHomeActionResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.ActionType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("action_type is required"))
	}
	if req.Msg.ItemKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item_key is required"))
	}

	var metadataJSON string
	if req.Msg.MetadataJson != nil {
		metadataJSON = *req.Msg.MetadataJson
	}

	if err := h.trackActionUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ActionType, req.Msg.ItemKey, metadataJSON); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeAction")
	}

	return connect.NewResponse(&knowledgehomev1.TrackHomeActionResponse{}), nil
}
