package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_projection_version_port"
	"alt/port/recall_candidate_port"
	"alt/port/summary_version_port"
	"alt/port/tag_set_version_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"alt/utils/textutil"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	projectorName = "knowledge-home-projector"
	batchSize     = 100
)

// KnowledgeProjectorJob returns a function suitable for the JobScheduler that
// processes knowledge events and projects them to read models.
func KnowledgeProjectorJob(
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	clearSupersedePort ...knowledge_home_port.ClearSupersedeStatePort,
) func(ctx context.Context) error {
	var clearPort knowledge_home_port.ClearSupersedeStatePort
	if len(clearSupersedePort) > 0 {
		clearPort = clearSupersedePort[0]
	}
	return func(ctx context.Context) error {
		// Resolve active projection version
		projectionVersion := 1
		if activeVersionPort != nil {
			v, err := activeVersionPort.GetActiveVersion(ctx)
			if err != nil {
				logger.Logger.ErrorContext(ctx, "failed to get active projection version, using default", "error", err)
			} else if v != nil {
				projectionVersion = v.Version
			}
		}
		return processKnowledgeEvents(ctx, eventsPort, checkpointPort, updateCheckpointPort, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, tagSetVersionPort, clearPort, projectionVersion)
	}
}

func processKnowledgeEvents(
	ctx context.Context,
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	clearSupersedePort knowledge_home_port.ClearSupersedeStatePort,
	projectionVersion int,
) error {
	// Get current checkpoint
	lastSeq, err := checkpointPort.GetProjectionCheckpoint(ctx, projectorName)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get projection checkpoint", "error", err)
		return fmt.Errorf("get checkpoint: %w", err)
	}

	// Fetch unprocessed events
	events, err := eventsPort.ListKnowledgeEventsSince(ctx, lastSeq, batchSize)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch knowledge events", "error", err)
		return fmt.Errorf("fetch events: %w", err)
	}

	if len(events) == 0 {
		// Heartbeat: touch checkpoint updated_at so freshness SLI stays accurate
		// even when no new events are arriving.
		if err := updateCheckpointPort.UpdateProjectionCheckpoint(ctx, projectorName, lastSeq); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to heartbeat projection checkpoint", "error", err)
		}
		return nil
	}

	logger.Logger.InfoContext(ctx, "processing knowledge events",
		"count", len(events), "from_seq", lastSeq)

	var maxSeq int64
	for _, event := range events {
		if err := projectEvent(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, tagSetVersionPort, clearSupersedePort, projectionVersion); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to project event",
				"error", err, "event_id", event.EventID, "event_type", event.EventType)
			// Continue with other events (best effort)
			continue
		}
		if event.EventSeq > maxSeq {
			maxSeq = event.EventSeq
		}
	}

	// Update checkpoint
	if maxSeq > 0 {
		if err := updateCheckpointPort.UpdateProjectionCheckpoint(ctx, projectorName, maxSeq); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to update projection checkpoint",
				"error", err, "max_seq", maxSeq)
			return fmt.Errorf("update checkpoint: %w", err)
		}
	}

	return nil
}

func projectEvent(
	ctx context.Context,
	event domain.KnowledgeEvent,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	clearSupersedePort knowledge_home_port.ClearSupersedeStatePort,
	projectionVersion int,
) error {
	switch event.EventType {
	case domain.EventArticleCreated:
		return projectArticleCreated(ctx, event, homeItemsPort, todayDigestPort, projectionVersion)
	case domain.EventSummaryVersionCreated:
		return projectSummaryVersionCreated(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, projectionVersion)
	case domain.EventTagSetVersionCreated:
		return projectTagSetVersionCreated(ctx, event, homeItemsPort, tagSetVersionPort, projectionVersion)
	case domain.EventHomeItemOpened:
		return projectHomeItemOpened(ctx, event, homeItemsPort, recallCandidatePort, clearSupersedePort, projectionVersion)
	case domain.EventHomeItemDismissed:
		return projectHomeItemDismissed(ctx, event, homeItemsPort, projectionVersion)
	case domain.EventSummarySuperseded:
		return projectSummarySuperseded(ctx, event, homeItemsPort, projectionVersion)
	case domain.EventTagSetSuperseded:
		return projectTagSetSuperseded(ctx, event, homeItemsPort, projectionVersion)
	case domain.EventReasonMerged:
		return projectReasonMerged(ctx, event, homeItemsPort, projectionVersion)
	default:
		// Unknown event types are silently skipped
		return nil
	}
}

// articleCreatedPayload is the expected payload for ArticleCreated events.
type articleCreatedPayload struct {
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	TenantID    string `json:"tenant_id"`
}

func projectArticleCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, todayDigestPort today_digest_port.UpsertTodayDigestPort, projectionVersion int) error {
	var payload articleCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ArticleCreated payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	now := time.Now()
	var publishedAt *time.Time
	if payload.PublishedAt != "" {
		t, err := time.Parse(time.RFC3339, payload.PublishedAt)
		if err == nil {
			publishedAt = &t
		}
	}

	// Calculate freshness score (newer = higher)
	score := 1.0
	if publishedAt != nil {
		hoursOld := time.Since(*publishedAt).Hours()
		if hoursOld < 24 {
			score = 1.0 - (hoursOld / 48.0) // decays to 0.5 over 24h
		} else {
			score = 0.5 / (hoursOld / 24.0) // further decay
		}
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &articleID,
		Title:             payload.Title,
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}},
		Score:             score,
		SummaryState:      domain.SummaryStatePending,
		FreshnessAt:       &now,
		PublishedAt:       publishedAt,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	if err := port.UpsertKnowledgeHomeItem(ctx, item); err != nil {
		return err
	}

	// Update today digest: increment new_articles
	if todayDigestPort != nil {
		digest := domain.TodayDigest{
			UserID:               userID,
			DigestDate:           now,
			NewArticles:          1,
			UnsummarizedArticles: 1,
			UpdatedAt:            now,
		}
		if err := todayDigestPort.UpsertTodayDigest(ctx, digest); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to update today digest for ArticleCreated", "error", err)
			// Non-fatal: don't fail the projection
		}
	}

	return nil
}

type summaryVersionPayload struct {
	SummaryVersionID string `json:"summary_version_id"`
	ArticleID        string `json:"article_id"`
}

const maxExcerptLen = 200

func projectSummaryVersionCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, todayDigestPort today_digest_port.UpsertTodayDigestPort, summaryVersionPort summary_version_port.GetSummaryVersionByIDPort, projectionVersion int) error {
	var payload summaryVersionPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SummaryVersionCreated payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	// Fetch summary text by the specific version ID from the event payload (reproject-safe).
	svID, err := uuid.Parse(payload.SummaryVersionID)
	if err != nil {
		return fmt.Errorf("parse summary_version_id: %w", err)
	}

	var summaryExcerpt string
	if summaryVersionPort != nil {
		sv, err := summaryVersionPort.GetSummaryVersionByID(ctx, svID)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to get summary version for excerpt", "error", err, "summary_version_id", svID)
		} else if sv.SummaryText != "" {
			summaryExcerpt = textutil.TruncateValidUTF8(sv.SummaryText, maxExcerptLen)
		}
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	// Determine summary state from excerpt
	summaryState := domain.SummaryStatePending
	if summaryExcerpt != "" {
		summaryState = domain.SummaryStateReady
	}

	whyReasons := []domain.WhyReason{{Code: domain.WhyNewUnread}}
	if summaryExcerpt != "" {
		whyReasons = append(whyReasons, domain.WhyReason{Code: domain.WhySummaryCompleted})
	}

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &articleID,
		Title:             "", // Preserved by merge-safe upsert
		SummaryExcerpt:    summaryExcerpt,
		SummaryState:      summaryState,
		WhyReasons:        whyReasons,
		Score:             0.8, // Boost for having a summary
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	if err := port.UpsertKnowledgeHomeItem(ctx, item); err != nil {
		return err
	}

	// Update today digest: increment summarized_articles
	if todayDigestPort != nil {
		summarizedArticles := 0
		unsummarizedDelta := 0
		if summaryState == domain.SummaryStateReady {
			summarizedArticles = 1
			unsummarizedDelta = -1
		}

		digest := domain.TodayDigest{
			UserID:               userID,
			DigestDate:           now,
			SummarizedArticles:   summarizedArticles,
			UnsummarizedArticles: unsummarizedDelta,
			UpdatedAt:            now,
		}
		if err := todayDigestPort.UpsertTodayDigest(ctx, digest); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to update today digest for SummaryVersionCreated", "error", err)
		}
	}

	return nil
}

type tagSetVersionPayload struct {
	TagSetVersionID string `json:"tag_set_version_id"`
	ArticleID       string `json:"article_id"`
}

// tagItem mirrors the JSON shape stored in TagsJSON.
// Supports both Go-default keys (Name/Confidence) and explicit json-tagged keys (name/confidence).
type tagItem struct {
	Name       string  `json:"name"`
	Confidence float32 `json:"confidence"`
}

// parseTagNames extracts tag names from TagsJSON, handling both
// {"Name":"x"} (Go default) and {"name":"x"} (json-tagged) key formats.
func parseTagNames(raw json.RawMessage) []string {
	// Try structured parse first (handles both key casings via json:"name")
	var items []tagItem
	if err := json.Unmarshal(raw, &items); err == nil {
		var names []string
		for _, t := range items {
			if t.Name != "" {
				names = append(names, t.Name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	// Fallback: parse as generic maps to handle uppercase keys
	var maps []map[string]interface{}
	if err := json.Unmarshal(raw, &maps); err == nil {
		var names []string
		for _, m := range maps {
			if name, ok := m["Name"].(string); ok && name != "" {
				names = append(names, name)
			} else if name, ok := m["name"].(string); ok && name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	return nil
}

func projectTagSetVersionCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort, projectionVersion int) error {
	var payload tagSetVersionPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal TagSetVersionCreated payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	// Read tags from the specific version referenced by the event
	var tags []string
	whyReasons := []domain.WhyReason{{Code: domain.WhyNewUnread}}

	if tagSetVersionPort != nil && payload.TagSetVersionID != "" {
		tsvID, parseErr := uuid.Parse(payload.TagSetVersionID)
		if parseErr == nil {
			tsv, tsvErr := tagSetVersionPort.GetTagSetVersionByID(ctx, tsvID)
			if tsvErr == nil && len(tsv.TagsJSON) > 0 {
				tags = parseTagNames(tsv.TagsJSON)
				if len(tags) > 0 {
					whyReasons = append(whyReasons, domain.WhyReason{
						Code: domain.WhyTagHotspot,
						Tag:  tags[0],
					})
				}
			} else if tsvErr != nil {
				logger.Logger.ErrorContext(ctx, "failed to get tag set version for projection", "error", tsvErr, "tag_set_version_id", tsvID)
			}
		}
	}

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            event.TenantID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &articleID,
		Title:             "", // Preserved by merge-safe upsert
		Tags:              tags,
		WhyReasons:        whyReasons,
		Score:             0.7,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	if event.UserID != nil {
		item.UserID = *event.UserID
	}

	return port.UpsertKnowledgeHomeItem(ctx, item)
}

type homeItemOpenedPayload struct {
	ItemKey string `json:"item_key"`
}

func projectHomeItemOpened(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort, clearSupersedePort knowledge_home_port.ClearSupersedeStatePort, projectionVersion int) error {
	var payload homeItemOpenedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal HomeItemOpened payload: %w", err)
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	// Reduce score for opened items (interaction suppression)
	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           payload.ItemKey,
		ItemType:          domain.ItemArticle,
		Title:             "",
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}},
		Score:             0.1, // Suppressed score
		LastInteractedAt:  &now,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	if err := port.UpsertKnowledgeHomeItem(ctx, item); err != nil {
		return err
	}

	// Clear supersede state on open (acknowledgement)
	if clearSupersedePort != nil {
		if err := clearSupersedePort.ClearSupersedeState(ctx, userID, payload.ItemKey, projectionVersion); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to clear supersede state on open", "error", err, "item_key", payload.ItemKey)
			// Non-fatal
		}
	}

	// Create recall candidate: eligible after 24h
	if recallCandidatePort != nil {
		eligibleAt := now.Add(24 * time.Hour)
		candidate := domain.RecallCandidate{
			UserID:            userID,
			ItemKey:           payload.ItemKey,
			RecallScore:       0.5,
			Reasons:           []domain.RecallReason{{Type: domain.ReasonOpenedNotRevisited, Description: "Opened but not revisited"}},
			FirstEligibleAt:   &eligibleAt,
			NextSuggestAt:     &eligibleAt,
			UpdatedAt:         now,
			ProjectionVersion: projectionVersion,
		}
		if err := recallCandidatePort.UpsertRecallCandidate(ctx, candidate); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to create recall candidate", "error", err)
			// Non-fatal
		}
	}

	return nil
}

type homeItemDismissedPayload struct {
	ItemKey string `json:"item_key"`
}

func projectHomeItemDismissed(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.DismissKnowledgeHomeItemPort, projectionVersion int) error {
	var payload homeItemDismissedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal HomeItemDismissed payload: %w", err)
	}
	if payload.ItemKey == "" {
		payload.ItemKey = event.AggregateID
	}
	if payload.ItemKey == "" {
		return fmt.Errorf("home item dismiss payload missing item_key")
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	dismissedAt := event.OccurredAt
	if dismissedAt.IsZero() {
		dismissedAt = time.Now()
	}

	return port.DismissKnowledgeHomeItem(ctx, userID, payload.ItemKey, projectionVersion, dismissedAt)
}

// ── Supersede projection handlers ──

type summarySupersededPayload struct {
	ArticleID              string `json:"article_id"`
	NewSummaryVersionID    string `json:"new_summary_version_id"`
	OldSummaryVersionID    string `json:"old_summary_version_id"`
	PreviousSummaryExcerpt string `json:"previous_summary_excerpt"`
}

func projectSummarySuperseded(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, projectionVersion int) error {
	var payload summarySupersededPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SummarySuperseded payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	prevRef, _ := json.Marshal(map[string]string{
		"previous_summary_excerpt": payload.PreviousSummaryExcerpt,
	})

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		SupersedeState:    domain.SupersedeSummaryUpdated,
		SupersededAt:      &now,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	return port.UpsertKnowledgeHomeItem(ctx, item)
}

type tagSetSupersededPayload struct {
	ArticleID          string   `json:"article_id"`
	NewTagSetVersionID string   `json:"new_tag_set_version_id"`
	OldTagSetVersionID string   `json:"old_tag_set_version_id"`
	PreviousTags       []string `json:"previous_tags"`
}

func projectTagSetSuperseded(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, projectionVersion int) error {
	var payload tagSetSupersededPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal TagSetSuperseded payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	prevRef, _ := json.Marshal(map[string][]string{
		"previous_tags": payload.PreviousTags,
	})

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		SupersedeState:    domain.SupersedeTagsUpdated,
		SupersededAt:      &now,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	return port.UpsertKnowledgeHomeItem(ctx, item)
}

type reasonMergedPayload struct {
	ArticleID        string   `json:"article_id"`
	ItemKey          string   `json:"item_key"`
	AddedCodes       []string `json:"added_codes"`
	PreviousWhyCodes []string `json:"previous_why_codes"`
}

func projectReasonMerged(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, projectionVersion int) error {
	var payload reasonMergedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ReasonMerged payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}

	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = fmt.Sprintf("article:%s", articleID)
	}

	prevRef, _ := json.Marshal(map[string][]string{
		"previous_why_codes": payload.PreviousWhyCodes,
	})

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           itemKey,
		ItemType:          domain.ItemArticle,
		SupersedeState:    domain.SupersedeReasonUpdated,
		SupersededAt:      &now,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	return port.UpsertKnowledgeHomeItem(ctx, item)
}
