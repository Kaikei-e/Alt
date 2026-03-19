package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_reproject_port"
	"alt/port/summary_version_port"
	"alt/port/tag_set_version_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	reprojectBatchSize         = 2000
	reprojectLoopSafetyMargin  = 3 * time.Second
)

// KnowledgeReprojectJob returns a function suitable for the JobScheduler that
// processes pending/running reproject runs. Designed to be called periodically.
func KnowledgeReprojectJob(
	listRunsPort knowledge_reproject_port.ListReprojectRunsPort,
	getRunPort knowledge_reproject_port.GetReprojectRunPort,
	updateRunPort knowledge_reproject_port.UpdateReprojectRunPort,
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
		knowledge_home_port.ClearSupersedeStatePort
	},
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processReprojectBatch(ctx, listRunsPort, getRunPort, updateRunPort, eventsPort, checkpointPort, updateCheckpointPort, homeItemsPort, todayDigestPort, summaryVersionPort, tagSetVersionPort)
	}
}

func processReprojectBatch(
	ctx context.Context,
	listRunsPort knowledge_reproject_port.ListReprojectRunsPort,
	_ knowledge_reproject_port.GetReprojectRunPort,
	updateRunPort knowledge_reproject_port.UpdateReprojectRunPort,
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	_ knowledge_projection_port.GetProjectionCheckpointPort,
	_ knowledge_projection_port.UpdateProjectionCheckpointPort,
	homeItemsPort interface {
		knowledge_home_port.UpsertKnowledgeHomeItemPort
		knowledge_home_port.DismissKnowledgeHomeItemPort
		knowledge_home_port.ClearSupersedeStatePort
	},
	_ today_digest_port.UpsertTodayDigestPort,
	summaryVersionPort summary_version_port.GetSummaryVersionByIDPort,
	tagSetVersionPort tag_set_version_port.GetTagSetVersionByIDPort,
) error {
	// Find pending or running reproject runs
	runs, err := listRunsPort.ListReprojectRuns(ctx, "", 10)
	if err != nil {
		return fmt.Errorf("list reproject runs: %w", err)
	}

	var activeRun *domain.ReprojectRun
	for i := range runs {
		if runs[i].Status == domain.ReprojectStatusPending || runs[i].Status == domain.ReprojectStatusRunning {
			activeRun = &runs[i]
			break
		}
	}

	if activeRun == nil {
		return nil // No active reproject runs
	}

	// If pending, transition to running
	if activeRun.Status == domain.ReprojectStatusPending {
		now := time.Now()
		activeRun.Status = domain.ReprojectStatusRunning
		activeRun.StartedAt = &now
		if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
			return fmt.Errorf("transition reproject to running: %w", err)
		}
		logger.Logger.InfoContext(ctx, "reproject run started",
			"run_id", activeRun.ReprojectRunID,
			"mode", activeRun.Mode,
			"to_version", activeRun.ToVersion)
	}

	// For dry_run mode, skip actual projection and go straight to validating
	if activeRun.Mode == domain.ReprojectModeDryRun {
		now := time.Now()
		activeRun.Status = domain.ReprojectStatusSwappable
		activeRun.FinishedAt = &now
		statsJSON, _ := json.Marshal(domain.ReprojectStats{
			EventsProcessed: 0,
			EventsTotal:     0,
		})
		activeRun.StatsJSON = statsJSON
		if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
			return fmt.Errorf("complete dry_run reproject: %w", err)
		}
		logger.Logger.InfoContext(ctx, "reproject dry_run completed",
			"run_id", activeRun.ReprojectRunID)
		return nil
	}

	targetVersion, err := parseReprojectVersion(activeRun.ToVersion)
	if err != nil {
		activeRun.Status = domain.ReprojectStatusFailed
		if updateErr := updateRunPort.UpdateReprojectRun(ctx, activeRun); updateErr != nil {
			logger.Logger.ErrorContext(ctx, "failed to persist reproject failure", "error", updateErr, "run_id", activeRun.ReprojectRunID)
		}
		return fmt.Errorf("parse reproject target version: %w", err)
	}

	// Process batches of events for the target version in a loop until
	// events are exhausted or the context deadline approaches.
	var checkpoint struct {
		LastEventSeq int64 `json:"last_event_seq"`
	}
	_ = json.Unmarshal(activeRun.CheckpointPayload, &checkpoint)

	var stats domain.ReprojectStats
	_ = json.Unmarshal(activeRun.StatsJSON, &stats)

	completed := false
	tickStart := time.Now()

	for {
		// Stop if context deadline is approaching
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < reprojectLoopSafetyMargin {
			break
		}

		events, err := eventsPort.ListKnowledgeEventsSince(ctx, checkpoint.LastEventSeq, reprojectBatchSize)
		if err != nil {
			return fmt.Errorf("fetch events for reproject: %w", err)
		}

		if len(events) == 0 {
			completed = true
			break
		}

		// Process events (project to target version)
		var processedCount int64
		var errorCount int64
		for _, event := range events {
			if err := projectEvent(ctx, event, homeItemsPort, nil, summaryVersionPort, nil, tagSetVersionPort, homeItemsPort, targetVersion); err != nil {
				errorCount++
				logger.Logger.ErrorContext(ctx, "failed to replay reproject event",
					"error", err,
					"run_id", activeRun.ReprojectRunID,
					"event_id", event.EventID,
					"event_type", event.EventType,
					"event_seq", event.EventSeq)
			}
			if event.EventSeq > checkpoint.LastEventSeq {
				checkpoint.LastEventSeq = event.EventSeq
			}
			processedCount++
		}

		stats.EventsProcessed += processedCount
		stats.ErrorCount += errorCount

		// Persist checkpoint after each batch
		checkpointJSON, _ := json.Marshal(checkpoint)
		activeRun.CheckpointPayload = checkpointJSON
		statsJSON, _ := json.Marshal(stats)
		activeRun.StatsJSON = statsJSON

		if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
			return fmt.Errorf("update reproject checkpoint: %w", err)
		}

		logger.Logger.InfoContext(ctx, "reproject batch processed",
			"run_id", activeRun.ReprojectRunID,
			"batch_size", len(events),
			"total_processed", stats.EventsProcessed,
			"elapsed", time.Since(tickStart).String())
	}

	if completed {
		now := time.Now()
		activeRun.Status = domain.ReprojectStatusSwappable
		activeRun.FinishedAt = &now
		// Final stats update
		statsJSON, _ := json.Marshal(stats)
		activeRun.StatsJSON = statsJSON
		if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
			return fmt.Errorf("complete reproject: %w", err)
		}
		logger.Logger.InfoContext(ctx, "reproject completed, ready for swap",
			"run_id", activeRun.ReprojectRunID,
			"total_processed", stats.EventsProcessed,
			"elapsed", time.Since(tickStart).String())
	}

	return nil
}

func parseReprojectVersion(version string) (int, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(version), "v"))
	if trimmed == "" {
		return 0, fmt.Errorf("empty projection version")
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid projection version %q: %w", version, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("projection version must be positive: %q", version)
	}
	return parsed, nil
}
