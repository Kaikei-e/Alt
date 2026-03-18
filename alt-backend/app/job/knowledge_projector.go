package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_projection_version_port"
	"alt/port/recall_candidate_port"
	"alt/port/summary_version_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
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
	homeItemsPort knowledge_home_port.UpsertKnowledgeHomeItemPort,
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	summaryVersionPort summary_version_port.GetLatestSummaryVersionPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
) func(ctx context.Context) error {
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
		return processKnowledgeEvents(ctx, eventsPort, checkpointPort, updateCheckpointPort, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, projectionVersion)
	}
}

func processKnowledgeEvents(
	ctx context.Context,
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort knowledge_home_port.UpsertKnowledgeHomeItemPort,
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetLatestSummaryVersionPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
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
		if err := projectEvent(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, projectionVersion); err != nil {
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
	homeItemsPort knowledge_home_port.UpsertKnowledgeHomeItemPort,
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetLatestSummaryVersionPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	projectionVersion int,
) error {
	switch event.EventType {
	case domain.EventArticleCreated:
		return projectArticleCreated(ctx, event, homeItemsPort, todayDigestPort, projectionVersion)
	case domain.EventSummaryVersionCreated:
		return projectSummaryVersionCreated(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, projectionVersion)
	case domain.EventTagSetVersionCreated:
		return projectTagSetVersionCreated(ctx, event, homeItemsPort, projectionVersion)
	case domain.EventHomeItemOpened:
		return projectHomeItemOpened(ctx, event, homeItemsPort, recallCandidatePort, projectionVersion)
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
			UserID:      userID,
			DigestDate:  now,
			NewArticles: 1,
			UpdatedAt:   now,
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

func projectSummaryVersionCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, todayDigestPort today_digest_port.UpsertTodayDigestPort, summaryVersionPort summary_version_port.GetLatestSummaryVersionPort, projectionVersion int) error {
	var payload summaryVersionPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SummaryVersionCreated payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	// Fetch summary text to generate excerpt
	var summaryExcerpt string
	if summaryVersionPort != nil {
		sv, err := summaryVersionPort.GetLatestSummaryVersion(ctx, articleID)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to get summary version for excerpt", "error", err, "article_id", articleID)
		} else if sv.SummaryText != "" {
			summaryExcerpt = sv.SummaryText
			if len(summaryExcerpt) > maxExcerptLen {
				summaryExcerpt = summaryExcerpt[:maxExcerptLen] + "…"
			}
		}
	}

	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
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
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}, {Code: domain.WhySummaryCompleted}},
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
		digest := domain.TodayDigest{
			UserID:              userID,
			DigestDate:          now,
			SummarizedArticles:  1,
			UpdatedAt:           now,
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

func projectTagSetVersionCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, projectionVersion int) error {
	var payload tagSetVersionPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal TagSetVersionCreated payload: %w", err)
	}

	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	now := time.Now()
	item := domain.KnowledgeHomeItem{
		UserID:            event.TenantID,
		TenantID:          event.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          domain.ItemArticle,
		PrimaryRefID:      &articleID,
		Title:             "",
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}},
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

func projectHomeItemOpened(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort, projectionVersion int) error {
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
