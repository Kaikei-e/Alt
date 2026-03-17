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

	"github.com/google/uuid"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/port/feature_flag_port"
	"alt/port/knowledge_event_port"
	"alt/usecase/archive_lens_usecase"
	"alt/usecase/create_lens_usecase"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/list_lenses_usecase"
	"alt/usecase/recall_dismiss_usecase"
	"alt/usecase/recall_rail_usecase"
	"alt/usecase/recall_snooze_usecase"
	"alt/usecase/select_lens_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/usecase/update_lens_usecase"
)

// Handler implements KnowledgeHomeServiceHandler.
type Handler struct {
	getHomeUsecase      *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	trackSeenUsecase    *track_home_seen_usecase.TrackHomeSeenUsecase
	trackActionUsecase  *track_home_action_usecase.TrackHomeActionUsecase
	recallRailUsecase   *recall_rail_usecase.RecallRailUsecase
	recallSnoozeUsecase *recall_snooze_usecase.RecallSnoozeUsecase
	recallDismissUsecase *recall_dismiss_usecase.RecallDismissUsecase
	createLensUsecase   *create_lens_usecase.CreateLensUsecase
	updateLensUsecase   *update_lens_usecase.UpdateLensUsecase
	listLensesUsecase   *list_lenses_usecase.ListLensesUsecase
	selectLensUsecase   *select_lens_usecase.SelectLensUsecase
	archiveLensUsecase  *archive_lens_usecase.ArchiveLensUsecase
	eventsPort          knowledge_event_port.ListKnowledgeEventsPort
	featureFlagPort     feature_flag_port.FeatureFlagPort
	logger              *slog.Logger
}

// Compile-time interface verification.
var _ knowledgehomev1connect.KnowledgeHomeServiceHandler = (*Handler)(nil)

// NewHandler creates a new KnowledgeHomeService handler.
func NewHandler(
	getHome *get_knowledge_home_usecase.GetKnowledgeHomeUsecase,
	trackSeen *track_home_seen_usecase.TrackHomeSeenUsecase,
	trackAction *track_home_action_usecase.TrackHomeActionUsecase,
	recallRail *recall_rail_usecase.RecallRailUsecase,
	recallSnooze *recall_snooze_usecase.RecallSnoozeUsecase,
	recallDismiss *recall_dismiss_usecase.RecallDismissUsecase,
	createLens *create_lens_usecase.CreateLensUsecase,
	updateLens *update_lens_usecase.UpdateLensUsecase,
	listLenses *list_lenses_usecase.ListLensesUsecase,
	selectLens *select_lens_usecase.SelectLensUsecase,
	archiveLens *archive_lens_usecase.ArchiveLensUsecase,
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	featureFlag feature_flag_port.FeatureFlagPort,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		getHomeUsecase:       getHome,
		trackSeenUsecase:     trackSeen,
		trackActionUsecase:   trackAction,
		recallRailUsecase:    recallRail,
		recallSnoozeUsecase:  recallSnooze,
		recallDismissUsecase: recallDismiss,
		createLensUsecase:    createLens,
		updateLensUsecase:    updateLens,
		listLensesUsecase:    listLenses,
		selectLensUsecase:    selectLens,
		archiveLensUsecase:   archiveLens,
		eventsPort:           eventsPort,
		featureFlagPort:      featureFlag,
		logger:               logger,
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

	// Convert items to proto
	protoItems := make([]*knowledgehomev1.KnowledgeHomeItem, 0, len(result.Items))
	for _, item := range result.Items {
		protoItems = append(protoItems, convertHomeItemToProto(item))
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
			domain.FlagRecallRail,
			domain.FlagLensV0,
			domain.FlagStreamUpdates,
			domain.FlagSupersedeUX,
		}
		for _, flag := range flags {
			featureFlags = append(featureFlags, &knowledgehomev1.FeatureFlagStatus{
				Name:    flag,
				Enabled: h.featureFlagPort.IsEnabled(flag, user.UserID),
			})
		}
	}

	resp := &knowledgehomev1.GetKnowledgeHomeResponse{
		TodayDigest:  digest,
		Items:        protoItems,
		NextCursor:   result.NextCursor,
		HasMore:      result.HasMore,
		DegradedMode: result.Degraded,
		GeneratedAt:  result.GeneratedAt.Format(time.RFC3339),
		FeatureFlags: featureFlags,
	}

	// Embed recall candidates if recall rail is enabled
	if h.featureFlagPort != nil && h.featureFlagPort.IsEnabled(domain.FlagRecallRail, user.UserID) && h.recallRailUsecase != nil {
		candidates, err := h.recallRailUsecase.Execute(ctx, user.UserID, 5)
		if err == nil {
			for _, c := range candidates {
				resp.RecallCandidates = append(resp.RecallCandidates, convertRecallCandidateToProto(c))
			}
		}
	}

	return connect.NewResponse(resp), nil
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

// GetRecallRail returns recall candidates for the user.
func (h *Handler) GetRecallRail(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetRecallRailRequest],
) (*connect.Response[knowledgehomev1.GetRecallRailResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagRecallRail, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("recall rail is not enabled for this user"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 5
	}

	candidates, err := h.recallRailUsecase.Execute(ctx, user.UserID, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetRecallRail")
	}

	protoCandidates := make([]*knowledgehomev1.RecallCandidate, 0, len(candidates))
	for _, c := range candidates {
		protoCandidates = append(protoCandidates, convertRecallCandidateToProto(c))
	}

	return connect.NewResponse(&knowledgehomev1.GetRecallRailResponse{
		Candidates: protoCandidates,
	}), nil
}

// TrackRecallAction records a recall action (snooze/dismiss/open).
func (h *Handler) TrackRecallAction(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackRecallActionRequest],
) (*connect.Response[knowledgehomev1.TrackRecallActionResponse], error) {
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

	switch req.Msg.ActionType {
	case "snooze":
		snoozeHours := 24
		if req.Msg.SnoozeHours != nil {
			snoozeHours = int(*req.Msg.SnoozeHours)
		}
		if err := h.recallSnoozeUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey, snoozeHours); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.snooze")
		}
	case "dismiss":
		if err := h.recallDismissUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.dismiss")
		}
	case "open":
		// Track as a regular home action
		if err := h.trackActionUsecase.Execute(ctx, user.UserID, user.TenantID, "open", req.Msg.ItemKey, ""); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.open")
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown action_type: %s", req.Msg.ActionType))
	}

	return connect.NewResponse(&knowledgehomev1.TrackRecallActionResponse{}), nil
}

// CreateLens creates a new saved viewpoint.
func (h *Handler) CreateLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.CreateLensRequest],
) (*connect.Response[knowledgehomev1.CreateLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("name is required"))
	}

	input := create_lens_usecase.CreateLensInput{
		UserID:      user.UserID,
		TenantID:    user.TenantID,
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
	}

	if req.Msg.Version != nil {
		input.QueryText = req.Msg.Version.QueryText
		input.TagIDs = req.Msg.Version.TagIds
		input.TimeWindow = req.Msg.Version.TimeWindow
		input.IncludeRecap = req.Msg.Version.IncludeRecap
		input.IncludePulse = req.Msg.Version.IncludePulse
		input.SortMode = req.Msg.Version.SortMode
	}

	result, err := h.createLensUsecase.Execute(ctx, input)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "CreateLens")
	}

	return connect.NewResponse(&knowledgehomev1.CreateLensResponse{
		Lens: convertLensToProto(result.Lens, &result.Version),
	}), nil
}

// UpdateLens creates a new version of an existing lens.
func (h *Handler) UpdateLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.UpdateLensRequest],
) (*connect.Response[knowledgehomev1.UpdateLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID, err := parseUUID(req.Msg.LensId, "lens_id")
	if err != nil {
		return nil, err
	}

	input := update_lens_usecase.UpdateLensInput{
		LensID: lensID,
		UserID: user.UserID,
	}

	if req.Msg.Version != nil {
		input.QueryText = req.Msg.Version.QueryText
		input.TagIDs = req.Msg.Version.TagIds
		input.TimeWindow = req.Msg.Version.TimeWindow
		input.IncludeRecap = req.Msg.Version.IncludeRecap
		input.IncludePulse = req.Msg.Version.IncludePulse
		input.SortMode = req.Msg.Version.SortMode
	}

	version, err := h.updateLensUsecase.Execute(ctx, input)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "UpdateLens")
	}

	return connect.NewResponse(&knowledgehomev1.UpdateLensResponse{
		Lens: &knowledgehomev1.Lens{
			LensId:         req.Msg.LensId,
			CurrentVersion: convertLensVersionToProto(*version),
		},
	}), nil
}

// DeleteLens archives a lens (soft delete).
func (h *Handler) DeleteLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.DeleteLensRequest],
) (*connect.Response[knowledgehomev1.DeleteLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID, err := parseUUID(req.Msg.LensId, "lens_id")
	if err != nil {
		return nil, err
	}

	if err := h.archiveLensUsecase.Execute(ctx, user.UserID, lensID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "DeleteLens")
	}

	return connect.NewResponse(&knowledgehomev1.DeleteLensResponse{}), nil
}

// ListLenses returns all active lenses for the user.
func (h *Handler) ListLenses(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.ListLensesRequest],
) (*connect.Response[knowledgehomev1.ListLensesResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lenses, err := h.listLensesUsecase.Execute(ctx, user.UserID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "ListLenses")
	}

	protoLenses := make([]*knowledgehomev1.Lens, 0, len(lenses))
	for _, l := range lenses {
		protoLenses = append(protoLenses, convertLensToProto(l, l.CurrentVersion))
	}

	return connect.NewResponse(&knowledgehomev1.ListLensesResponse{
		Lenses: protoLenses,
	}), nil
}

// SelectLens sets the active lens for the user.
func (h *Handler) SelectLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.SelectLensRequest],
) (*connect.Response[knowledgehomev1.SelectLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID, err := parseUUID(req.Msg.LensId, "lens_id")
	if err != nil {
		return nil, err
	}

	if err := h.selectLensUsecase.Execute(ctx, user.UserID, lensID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "SelectLens")
	}

	return connect.NewResponse(&knowledgehomev1.SelectLensResponse{}), nil
}

// StreamKnowledgeHomeUpdates streams real-time updates for the home feed.
func (h *Handler) StreamKnowledgeHomeUpdates(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.StreamKnowledgeHomeUpdatesRequest],
	stream *connect.ServerStream[knowledgehomev1.StreamKnowledgeHomeUpdatesResponse],
) error {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagStreamUpdates, user.UserID) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("stream updates is not enabled for this user"))
	}

	h.logger.InfoContext(ctx, "alt.knowledge_home.stream_started",
		"user_id", user.UserID)

	updateTicker := time.NewTicker(5 * time.Second)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	var lastSeq int64

	for {
		select {
		case <-ctx.Done():
			h.logger.InfoContext(ctx, "alt.knowledge_home.stream_ended",
				"user_id", user.UserID, "reason", ctx.Err())
			return nil

		case <-updateTicker.C:
			if h.eventsPort == nil {
				continue
			}
			events, err := h.eventsPort.ListKnowledgeEventsSince(ctx, lastSeq, 50)
			if err != nil {
				h.logger.ErrorContext(ctx, "stream: failed to fetch events", "error", err)
				continue
			}
			for _, event := range events {
				update := &knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
					EventType:  event.EventType,
					OccurredAt: event.OccurredAt.Format(time.RFC3339),
				}
				if err := stream.Send(update); err != nil {
					return err
				}
				if event.EventSeq > lastSeq {
					lastSeq = event.EventSeq
				}
			}

		case <-heartbeatTicker.C:
			if err := stream.Send(&knowledgehomev1.StreamKnowledgeHomeUpdatesResponse{
				EventType:  "heartbeat",
				OccurredAt: time.Now().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
}

// StreamRecallRailUpdates streams real-time updates for the recall rail.
func (h *Handler) StreamRecallRailUpdates(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.StreamRecallRailUpdatesRequest],
	stream *connect.ServerStream[knowledgehomev1.StreamRecallRailUpdatesResponse],
) error {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagRecallRail, user.UserID) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("recall rail is not enabled for this user"))
	}

	h.logger.InfoContext(ctx, "alt.recall_rail.stream_started",
		"user_id", user.UserID)

	updateTicker := time.NewTicker(30 * time.Second)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-updateTicker.C:
			if h.recallRailUsecase == nil {
				continue
			}
			candidates, err := h.recallRailUsecase.Execute(ctx, user.UserID, 5)
			if err != nil {
				h.logger.ErrorContext(ctx, "recall stream: failed to fetch candidates", "error", err)
				continue
			}
			for _, c := range candidates {
				update := &knowledgehomev1.StreamRecallRailUpdatesResponse{
					EventType:  "candidate_updated",
					Candidate:  convertRecallCandidateToProto(c),
					OccurredAt: time.Now().Format(time.RFC3339),
				}
				if err := stream.Send(update); err != nil {
					return err
				}
			}

		case <-heartbeatTicker.C:
			if err := stream.Send(&knowledgehomev1.StreamRecallRailUpdatesResponse{
				EventType:  "heartbeat",
				OccurredAt: time.Now().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
}

// --- Proto conversion helpers ---

func convertHomeItemToProto(item domain.KnowledgeHomeItem) *knowledgehomev1.KnowledgeHomeItem {
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

	// Supersede info
	if item.SupersedeState != "" {
		info := &knowledgehomev1.SupersedeInfo{
			State: item.SupersedeState,
		}
		if item.SupersededAt != nil {
			info.SupersededAt = item.SupersededAt.Format(time.RFC3339)
		}
		protoItem.SupersedeInfo = info
	}

	return protoItem
}

func convertRecallCandidateToProto(c domain.RecallCandidate) *knowledgehomev1.RecallCandidate {
	proto := &knowledgehomev1.RecallCandidate{
		ItemKey:     c.ItemKey,
		RecallScore: c.RecallScore,
	}

	if c.FirstEligibleAt != nil {
		proto.FirstEligibleAt = c.FirstEligibleAt.Format(time.RFC3339)
	}
	if c.NextSuggestAt != nil {
		proto.NextSuggestAt = c.NextSuggestAt.Format(time.RFC3339)
	}

	for _, r := range c.Reasons {
		protoReason := &knowledgehomev1.RecallReason{
			Type:        r.Type,
			Description: r.Description,
		}
		if r.SourceItemKey != "" {
			protoReason.SourceItemKey = &r.SourceItemKey
		}
		proto.Reasons = append(proto.Reasons, protoReason)
	}

	if c.Item != nil {
		proto.Item = convertHomeItemToProto(*c.Item)
	}

	return proto
}

func convertLensToProto(l domain.KnowledgeLens, v *domain.KnowledgeLensVersion) *knowledgehomev1.Lens {
	lens := &knowledgehomev1.Lens{
		LensId:      l.LensID.String(),
		Name:        l.Name,
		Description: l.Description,
		CreatedAt:   l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   l.UpdatedAt.Format(time.RFC3339),
	}
	if v != nil {
		lens.CurrentVersion = convertLensVersionToProto(*v)
	}
	return lens
}

func convertLensVersionToProto(v domain.KnowledgeLensVersion) *knowledgehomev1.LensVersion {
	return &knowledgehomev1.LensVersion{
		VersionId:    v.LensVersionID.String(),
		QueryText:    v.QueryText,
		TagIds:       v.TagIDs,
		TimeWindow:   v.TimeWindow,
		IncludeRecap: v.IncludeRecap,
		IncludePulse: v.IncludePulse,
		SortMode:     v.SortMode,
	}
}

func parseUUID(s string, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid %s: %w", field, err))
	}
	return id, nil
}
