package job

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// outboxPruneRepository abstracts the outbox prune capability.
//
// Backed by datahub_gateway.OutboxGateway since ADR-000954 Wave 3: the DELETE
// runs on alt-data-hub, and the retention window below travels with the
// request rather than being a constant the provider holds.
type outboxPruneRepository interface {
	Prune(ctx context.Context, olderThan time.Duration) (int64, error)
}

// outboxPruneRetention is how long a PROCESSED outbox_events row is kept
// before deletion. FAILED rows are not pruned by this job at all — the SQL
// behind Prune (save_outbox_event_driver.go PruneOutboxEvents) only ever
// matched status='PROCESSED', so they are retained indefinitely as the
// append-first audit trail for a delivery incident until an operator clears
// them deliberately. 7 days gives ample time to investigate a PROCESSED row
// before it is gone.
const outboxPruneRetention = 7 * 24 * time.Hour

// OutboxPruneJob returns a JobScheduler function that deletes terminal
// (PROCESSED) outbox_events rows older than outboxPruneRetention.
//
// PruneOutboxEvents was implemented in save_outbox_event_driver.go (finding
// [13]) but never wired to any scheduled job: the outbox-worker (5s tick)
// only transitions PENDING -> PROCESSED/FAILED and never deletes, so
// outbox_events grew unbounded — the same "implemented but never wired"
// lifecycle-management gap already seen for search-indexer's task DB
// (PM-047-recurrence).
//
// repo is a required composition-root dependency (job/registry.go always
// wires container.AltDBRepository here, unconditionally — there is no
// feature flag that legitimately leaves it nil). A nil repo can only be a DI
// wiring bug, so this panics at construction time instead of silently
// no-op'ing on every scheduled tick forever (CLAUDE.md rule 8).
func OutboxPruneJob(repo outboxPruneRepository) func(ctx context.Context) error {
	if repo == nil {
		panic("outbox-prune: outbox repository is nil — must be wired unconditionally at composition root (see .claude/rules/di-wiring.md)")
	}
	return func(ctx context.Context) error {
		pruned, err := repo.Prune(ctx, outboxPruneRetention)
		if err != nil {
			return fmt.Errorf("prune outbox events: %w", err)
		}
		slog.InfoContext(ctx, "outbox-prune: completed", "pruned", pruned, "retention", outboxPruneRetention.String())
		return nil
	}
}
