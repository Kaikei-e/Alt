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
	altotel "alt/utils/otel"
	"alt/utils/textutil"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	projectorName             = "knowledge-home-projector"
	batchSize                 = 100
	projectorLoopSafetyMargin = 250 * time.Millisecond
)

// KnowledgeProjectorConfig configures the knowledge projector.
type KnowledgeProjectorConfig struct {
	BatchSize int
	Metrics   *altotel.KnowledgeHomeMetrics
}

// KnowledgeProjectorJob returns a function suitable for the JobScheduler that
// processes knowledge events and projects them to read models.
func KnowledgeProjectorJob(
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
		knowledge_home_port.PatchKnowledgeHomeItemURLPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	clearSupersedePort ...knowledge_home_port.ClearSupersedeStatePort,
) func(ctx context.Context) error {
	return KnowledgeProjectorJobWithConfig(
		eventsPort,
		checkpointPort,
		updateCheckpointPort,
		homeItemsPort,
		todayDigestPort,
		activeVersionPort,
		summaryVersionPort,
		recallCandidatePort,
		tagSetVersionPort,
		KnowledgeProjectorConfig{BatchSize: batchSize},
		clearSupersedePort...,
	)
}

func KnowledgeProjectorJobWithConfig(
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
		knowledge_home_port.PatchKnowledgeHomeItemURLPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	config KnowledgeProjectorConfig,
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
				logger.ErrorContext(ctx, "failed to get active projection version, using default", "error", err)
			} else if v != nil {
				projectionVersion = v.Version
			}
		}
		effectiveBatchSize := config.BatchSize
		if effectiveBatchSize <= 0 {
			effectiveBatchSize = batchSize
		}
		return processKnowledgeEvents(ctx, eventsPort, checkpointPort, updateCheckpointPort, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, tagSetVersionPort, clearPort, projectionVersion, effectiveBatchSize, config.Metrics)
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
		knowledge_home_port.PatchKnowledgeHomeItemURLPort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	recallCandidatePort recall_candidate_port.UpsertRecallCandidatePort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
	clearSupersedePort knowledge_home_port.ClearSupersedeStatePort,
	projectionVersion int,
	batchLimit int,
	metrics *altotel.KnowledgeHomeMetrics,
) error {
	// Get current checkpoint
	lastSeq, err := checkpointPort.GetProjectionCheckpoint(ctx, projectorName)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get projection checkpoint", "error", err)
		return fmt.Errorf("get checkpoint: %w", err)
	}

	processedAny := false
	for {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < projectorLoopSafetyMargin {
			break
		}

		events, err := eventsPort.ListKnowledgeEventsSince(ctx, lastSeq, batchLimit)
		if err != nil {
			logger.ErrorContext(ctx, "failed to fetch knowledge events", "error", err)
			return fmt.Errorf("fetch events: %w", err)
		}

		if len(events) == 0 {
			if !processedAny {
				// Heartbeat: touch checkpoint updated_at so freshness SLI stays accurate
				// even when no new events are arriving.
				if err := updateCheckpointPort.UpdateProjectionCheckpoint(ctx, projectorName, lastSeq); err != nil {
					logger.ErrorContext(ctx, "failed to heartbeat projection checkpoint", "error", err)
				}
			}
			return nil
		}

		processedAny = true
		batchStart := time.Now()
		logger.InfoContext(ctx, "processing knowledge events",
			"count", len(events), "from_seq", lastSeq)

		var maxSeq int64
		var errorCount int64
		var projectionErr error
		for _, event := range events {
			if err := projectEvent(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, recallCandidatePort, tagSetVersionPort, clearSupersedePort, projectionVersion); err != nil {
				logger.ErrorContext(ctx, "failed to project event",
					"error", err, "event_id", event.EventID, "event_type", event.EventType)
				errorCount++
				projectionErr = fmt.Errorf("project event %s (seq=%d): %w", event.EventType, event.EventSeq, err)
				// Stop processing: do not advance checkpoint past the failed event
				// so it will be retried on the next cycle.
				break
			}
			if event.EventSeq > maxSeq {
				maxSeq = event.EventSeq
			}
		}

		// Record batch metrics
		if metrics != nil {
			batchDuration := float64(time.Since(batchStart).Milliseconds())
			metrics.ProjectorEventsProcessed.Add(ctx, int64(len(events)))
			metrics.ProjectorBatchDurationMs.Record(ctx, batchDuration)
			if metrics.Snapshot != nil {
				for range len(events) {
					metrics.Snapshot.RecordProjectorEvent()
				}
				metrics.Snapshot.RecordProjectorBatch(batchDuration)
			}
			if errorCount > 0 {
				metrics.ProjectorErrors.Add(ctx, errorCount)
				if metrics.Snapshot != nil {
					for range errorCount {
						metrics.Snapshot.RecordProjectorError()
					}
				}
			}
		}

		if maxSeq > 0 {
			if err := updateCheckpointPort.UpdateProjectionCheckpoint(ctx, projectorName, maxSeq); err != nil {
				logger.ErrorContext(ctx, "failed to update projection checkpoint",
					"error", err, "max_seq", maxSeq)
				return fmt.Errorf("update checkpoint: %w", err)
			}
			lastSeq = maxSeq

			// Record projector lag: time since the latest event was created
			if metrics != nil {
				latestEvent := events[len(events)-1]
				lag := time.Since(latestEvent.OccurredAt).Seconds()
				metrics.ProjectorLagSeconds.Record(ctx, lag)
				if metrics.Snapshot != nil {
					metrics.Snapshot.RecordProjectorLag(lag)
				}
			}
		}

		if projectionErr != nil {
			return projectionErr
		}

		if len(events) < batchLimit {
			return nil
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
		knowledge_home_port.PatchKnowledgeHomeItemURLPort
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
	case domain.EventArticleUrlBackfilled:
		return projectArticleUrlBackfilled(ctx, event, homeItemsPort, projectionVersion)
	case domain.EventSummaryVersionCreated:
		return projectSummaryVersionCreated(ctx, event, homeItemsPort, todayDigestPort, summaryVersionPort, projectionVersion)
	case domain.EventTagSetVersionCreated:
		return projectTagSetVersionCreated(ctx, event, homeItemsPort, todayDigestPort, tagSetVersionPort, projectionVersion)
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

func projectArticleCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, todayDigestPort today_digest_port.UpsertTodayDigestPort, projectionVersion int) error {
	// The wire schema is owned by domain.ArticleCreatedPayload — the same
	// struct all 3 producers (outbox_worker / connect/v2/internal /
	// knowledge_backfill_job) marshal through. Keeping a separate consumer
	// struct here is what allowed PM-2026-041's drift to slip in; the
	// shared struct is now the single source of truth.
	var payload domain.ArticleCreatedPayload
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
		URL:               payload.URL,
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
			logger.ErrorContext(ctx, "failed to update today digest for ArticleCreated", "error", err)
			// Non-fatal: don't fail the projection
		}
	}

	return nil
}

// projectArticleUrlBackfilled handles the corrective ArticleUrlBackfilled
// event by patching only the `url` column of the matching
// knowledge_home_items row. ADR-000867 / docs/glossary/ubiquitous-language.md.
//
// Defense-in-depth: URL scheme is allowlisted to {http, https} both here
// and at the sovereign-side WHERE clause. Empty URL is also rejected
// at both layers. The corrective event itself comes from a vetted
// admin-authenticated emitter that reads the producer-side `articles.url`
// (a stable resource), so dangerous schemes should never get this far,
// but we treat the projector as untrusted boundary input regardless.
func projectArticleUrlBackfilled(
	ctx context.Context,
	event domain.KnowledgeEvent,
	port knowledge_home_port.PatchKnowledgeHomeItemURLPort,
	projectionVersion int,
) error {
	var payload domain.ArticleUrlBackfilledPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ArticleUrlBackfilled payload: %w", err)
	}
	if !isHTTPURL(payload.URL) {
		// Skip silently: the corrective event carries an unusable URL.
		// Logged at debug; the row stays with the existing (possibly
		// empty) url so downstream FE shows the Archived kicker rather
		// than smuggling a dangerous href.
		logger.WarnContext(ctx, "skipping ArticleUrlBackfilled with non-HTTP URL",
			"event_id", event.EventID, "article_id", payload.ArticleID)
		return nil
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}
	userID := event.TenantID
	if event.UserID != nil {
		userID = *event.UserID
	}
	itemKey := fmt.Sprintf("article:%s", articleID)
	if err := port.PatchKnowledgeHomeItemURL(ctx, userID, itemKey, projectionVersion, payload.URL); err != nil {
		return fmt.Errorf("patch knowledge_home_items.url: %w", err)
	}
	return nil
}

// isHTTPURL allowlist. Mirrors the FE-side safeArticleHref guard so a
// dangerous scheme rejected on the FE never sneaks back via the corrective
// event. Returns false for empty, malformed, or non-(http|https) URLs.
func isHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
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
			return fmt.Errorf("get summary version %s for excerpt: %w", svID, err)
		}
		if sv.SummaryText != "" {
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
			logger.ErrorContext(ctx, "failed to update today digest for SummaryVersionCreated", "error", err)
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

func projectTagSetVersionCreated(ctx context.Context, event domain.KnowledgeEvent, port knowledge_home_port.UpsertKnowledgeHomeItemPort, todayDigestPort today_digest_port.UpsertTodayDigestPort, tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort, projectionVersion int) error {
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
				// tag_hotspot is NOT assigned here — trending detection
				// is done at read time in GetKnowledgeHomeUsecase.
			} else if tsvErr != nil {
				logger.ErrorContext(ctx, "failed to get tag set version for projection", "error", tsvErr, "tag_set_version_id", tsvID)
			}
		}
	}

	now := time.Now()
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
		Title:             "", // Preserved by merge-safe upsert
		Tags:              tags,
		WhyReasons:        whyReasons,
		Score:             0.7,
		GeneratedAt:       now,
		UpdatedAt:         now,
		ProjectionVersion: projectionVersion,
	}

	if err := port.UpsertKnowledgeHomeItem(ctx, item); err != nil {
		return err
	}

	// Surface tags into today_digest_view.top_tags_json. The projection table
	// merges via COALESCE(NULLIF(EXCLUDED.top_tags_json, '[]'::jsonb), …) so
	// sending an empty list preserves whatever was previously written.
	if todayDigestPort != nil && len(tags) > 0 {
		digest := domain.TodayDigest{
			UserID:     userID,
			DigestDate: now,
			TopTags:    tags,
			UpdatedAt:  now,
		}
		if err := todayDigestPort.UpsertTodayDigest(ctx, digest); err != nil {
			logger.ErrorContext(ctx, "failed to update today digest top_tags from TagSetVersionCreated", "error", err)
			// Non-fatal: home item upsert already succeeded.
		}
	}

	return nil
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
	// Use event.OccurredAt for reproject-safety (immutable data model invariant)
	eventTime := event.OccurredAt
	item := domain.KnowledgeHomeItem{
		UserID:            userID,
		TenantID:          event.TenantID,
		ItemKey:           payload.ItemKey,
		ItemType:          domain.ItemArticle,
		Title:             "",
		WhyReasons:        []domain.WhyReason{{Code: domain.WhyNewUnread}},
		Score:             0.1, // Suppressed score
		LastInteractedAt:  &eventTime,
		GeneratedAt:       time.Now(),
		UpdatedAt:         time.Now(),
		ProjectionVersion: projectionVersion,
	}

	if err := port.UpsertKnowledgeHomeItem(ctx, item); err != nil {
		return err
	}

	// Clear supersede state on open (acknowledgement)
	if clearSupersedePort != nil {
		if err := clearSupersedePort.ClearSupersedeState(ctx, userID, payload.ItemKey, projectionVersion); err != nil {
			logger.ErrorContext(ctx, "failed to clear supersede state on open", "error", err, "item_key", payload.ItemKey)
			// Non-fatal
		}
	}

	// Create recall candidate: eligible after 1h (reproject-safe: based on event time)
	if recallCandidatePort != nil {
		eligibleAt := eventTime.Add(1 * time.Hour)
		candidate := domain.RecallCandidate{
			UserID:            userID,
			ItemKey:           payload.ItemKey,
			RecallScore:       0.5,
			Reasons:           []domain.RecallReason{{Type: domain.ReasonOpenedNotRevisited, Description: "Opened but not revisited"}},
			FirstEligibleAt:   &eligibleAt,
			NextSuggestAt:     &eligibleAt,
			UpdatedAt:         time.Now(),
			ProjectionVersion: projectionVersion,
		}
		if err := recallCandidatePort.UpsertRecallCandidate(ctx, candidate); err != nil {
			logger.ErrorContext(ctx, "failed to create recall candidate", "error", err)
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
		Tags:              []string{}, // must be empty slice, not nil — symmetry with projectTagSetSuperseded
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
		Tags:              []string{}, // must be empty slice, not nil — nil serializes to "null" JSON which overwrites existing tags
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
