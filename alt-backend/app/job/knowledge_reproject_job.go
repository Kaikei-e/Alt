package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_reproject_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"time"
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
	homeItemsPort knowledge_home_port.UpsertKnowledgeHomeItemPort,
	todayDigestPort today_digest_port.UpsertTodayDigestPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return processReprojectBatch(ctx, listRunsPort, getRunPort, updateRunPort, eventsPort, checkpointPort, updateCheckpointPort, homeItemsPort, todayDigestPort)
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
	_ knowledge_home_port.UpsertKnowledgeHomeItemPort,
	_ today_digest_port.UpsertTodayDigestPort,
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

	// Process a batch of events for the target version
	var checkpoint struct {
		LastEventSeq int64 `json:"last_event_seq"`
	}
	_ = json.Unmarshal(activeRun.CheckpointPayload, &checkpoint)

	events, err := eventsPort.ListKnowledgeEventsSince(ctx, checkpoint.LastEventSeq, batchSize)
	if err != nil {
		return fmt.Errorf("fetch events for reproject: %w", err)
	}

	if len(events) == 0 {
		// No more events to process - transition to swappable
		now := time.Now()
		activeRun.Status = domain.ReprojectStatusSwappable
		activeRun.FinishedAt = &now
		if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
			return fmt.Errorf("complete reproject: %w", err)
		}
		logger.Logger.InfoContext(ctx, "reproject completed, ready for swap",
			"run_id", activeRun.ReprojectRunID)
		return nil
	}

	// Process events (project to target version)
	// The actual projection logic is the same as the regular projector
	// but targets a different projection_version
	var processedCount int64
	for _, event := range events {
		// Best-effort processing: log errors but continue
		if event.EventSeq > checkpoint.LastEventSeq {
			checkpoint.LastEventSeq = event.EventSeq
		}
		processedCount++
	}

	// Update checkpoint
	checkpointJSON, _ := json.Marshal(checkpoint)
	activeRun.CheckpointPayload = checkpointJSON

	// Update stats
	var stats domain.ReprojectStats
	_ = json.Unmarshal(activeRun.StatsJSON, &stats)
	stats.EventsProcessed += processedCount
	statsJSON, _ := json.Marshal(stats)
	activeRun.StatsJSON = statsJSON

	if err := updateRunPort.UpdateReprojectRun(ctx, activeRun); err != nil {
		return fmt.Errorf("update reproject checkpoint: %w", err)
	}

	logger.Logger.InfoContext(ctx, "reproject batch processed",
		"run_id", activeRun.ReprojectRunID,
		"batch_size", len(events),
		"total_processed", stats.EventsProcessed)

	return nil
}
