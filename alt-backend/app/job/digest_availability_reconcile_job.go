package job

import (
	"alt/domain"
	"alt/port/knowledge_home_port"
	"alt/port/recap_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// DigestAvailabilityReconcileJob returns a scheduled function that reconciles
// Recap and Evening Pulse availability into today_digest_view.
// Users are dynamically discovered via ListDistinctUserIDsPort.
func DigestAvailabilityReconcileJob(
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	recapPort recap_port.RecapPort,
	digestPort today_digest_port.UpsertTodayDigestPort,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return digestAvailabilityReconcile(ctx, listUsersPort, recapPort, digestPort)
	}
}

func digestAvailabilityReconcile(
	ctx context.Context,
	listUsersPort knowledge_home_port.ListDistinctUserIDsPort,
	recapPort recap_port.RecapPort,
	digestPort today_digest_port.UpsertTodayDigestPort,
) error {
	userIDs, err := listUsersPort.ListDistinctUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("digest availability reconcile: list distinct user IDs: %w", err)
	}

	if len(userIDs) == 0 {
		return nil
	}

	now := time.Now()
	today := now.Format("2006-01-02")

	// Check recap availability (global, once)
	recapAvailable := false
	recapSkip := false
	_, err = recapPort.GetSevenDayRecap(ctx)
	if err == nil {
		recapAvailable = true
	} else if errors.Is(err, domain.ErrRecapNotFound) {
		recapAvailable = false
	} else {
		recapSkip = true
		logger.Logger.WarnContext(ctx, "digest availability reconcile: recap check failed, skipping",
			"error", err)
	}

	// Check pulse availability (global, once)
	pulseAvailable := false
	pulseSkip := false
	pulse, err := recapPort.GetEveningPulse(ctx, today)
	if err == nil {
		pulseAvailable = pulse.Status != domain.PulseStatusError
	} else if errors.Is(err, domain.ErrEveningPulseNotFound) {
		pulseAvailable = false
	} else {
		pulseSkip = true
		logger.Logger.WarnContext(ctx, "digest availability reconcile: pulse check failed, skipping",
			"error", err)
	}

	// If both checks failed with transient errors, nothing useful to write
	if recapSkip && pulseSkip {
		logger.Logger.WarnContext(ctx, "digest availability reconcile: both checks failed, skipping upsert")
		return nil
	}

	logger.Logger.InfoContext(ctx, "digest availability reconcile: updating digests",
		slog.Int("user_count", len(userIDs)),
		slog.Bool("recap_available", recapAvailable),
		slog.Bool("pulse_available", pulseAvailable),
		slog.Bool("recap_skipped", recapSkip),
		slog.Bool("pulse_skipped", pulseSkip),
	)

	for _, userID := range userIDs {
		digest := domain.TodayDigest{
			UserID:                userID,
			DigestDate:            now,
			WeeklyRecapAvailable:  recapAvailable,
			EveningPulseAvailable: pulseAvailable,
			UpdatedAt:             now,
		}

		if err := digestPort.UpsertTodayDigest(ctx, digest); err != nil {
			logger.Logger.ErrorContext(ctx, "digest availability reconcile: upsert failed",
				"error", err, "user_id", userID)
			continue
		}
	}

	return nil
}
