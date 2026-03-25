// Package knowledge_home provides the Connect-RPC handler for KnowledgeHomeService.
package knowledge_home

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"

	"github.com/google/uuid"

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
	altotel "alt/utils/otel"
)

// streamHighWaterMark is the threshold above which coalesced events
// are collapsed into a single digest_changed event to avoid flooding
// the client with too many individual updates in one batch.
const streamHighWaterMark = 10

// Handler implements KnowledgeHomeServiceHandler.
type Handler struct {
	getHomeUsecase       *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	trackSeenUsecase     *track_home_seen_usecase.TrackHomeSeenUsecase
	trackActionUsecase   *track_home_action_usecase.TrackHomeActionUsecase
	recallRailUsecase    *recall_rail_usecase.RecallRailUsecase
	recallSnoozeUsecase  *recall_snooze_usecase.RecallSnoozeUsecase
	recallDismissUsecase *recall_dismiss_usecase.RecallDismissUsecase
	createLensUsecase    *create_lens_usecase.CreateLensUsecase
	updateLensUsecase    *update_lens_usecase.UpdateLensUsecase
	listLensesUsecase    *list_lenses_usecase.ListLensesUsecase
	selectLensUsecase    *select_lens_usecase.SelectLensUsecase
	archiveLensUsecase   *archive_lens_usecase.ArchiveLensUsecase
	eventsPort           knowledge_event_port.ListKnowledgeEventsPort
	eventsForUserPort    knowledge_event_port.ListKnowledgeEventsForUserPort
	latestSeqPort        knowledge_event_port.LatestKnowledgeEventSeqForUserPort
	featureFlagPort      feature_flag_port.FeatureFlagPort
	metrics              *altotel.KnowledgeHomeMetrics
	logger               *slog.Logger
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
	eventsForUserPort knowledge_event_port.ListKnowledgeEventsForUserPort,
	featureFlag feature_flag_port.FeatureFlagPort,
	metrics *altotel.KnowledgeHomeMetrics,
	logger *slog.Logger,
) *Handler {
	var latestSeqPort knowledge_event_port.LatestKnowledgeEventSeqForUserPort
	if provider, ok := eventsForUserPort.(knowledge_event_port.LatestKnowledgeEventSeqForUserPort); ok {
		latestSeqPort = provider
	}

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
		eventsForUserPort:    eventsForUserPort,
		latestSeqPort:        latestSeqPort,
		featureFlagPort:      featureFlag,
		metrics:              metrics,
		logger:               logger,
	}
}

// --- Proto conversion helpers ---

func convertHomeItemToProto(item domain.KnowledgeHomeItem) *knowledgehomev1.KnowledgeHomeItem {
	protoItem := &knowledgehomev1.KnowledgeHomeItem{
		ItemKey:      item.ItemKey,
		ItemType:     item.ItemType,
		Title:        item.Title,
		Tags:         item.Tags,
		Score:        item.Score,
		SummaryState: item.SummaryState,
		Link:         item.Link,
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
		// Parse previous_ref_json for detail fields
		if item.PreviousRefJSON != "" {
			var prevRef struct {
				PreviousSummaryExcerpt string   `json:"previous_summary_excerpt"`
				PreviousTags           []string `json:"previous_tags"`
				PreviousWhyCodes       []string `json:"previous_why_codes"`
			}
			if json.Unmarshal([]byte(item.PreviousRefJSON), &prevRef) == nil {
				if prevRef.PreviousSummaryExcerpt != "" {
					info.PreviousSummaryExcerpt = &prevRef.PreviousSummaryExcerpt
				}
				if len(prevRef.PreviousTags) > 0 {
					info.PreviousTags = prevRef.PreviousTags
				}
				if len(prevRef.PreviousWhyCodes) > 0 {
					info.PreviousWhyCodes = prevRef.PreviousWhyCodes
				}
			}
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
		SourceIds:    v.SourceIDs,
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
