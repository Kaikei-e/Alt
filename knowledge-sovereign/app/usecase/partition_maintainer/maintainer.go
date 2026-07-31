// Package partition_maintainer keeps the monthly range partitions of the
// append-only event tables created ahead of time.
//
// Migrations 00006/00007 created six months each and stopped at
// TO ('2026-05-01'); ADR-000584 Decision 4 rejected pg_partman in favour of
// generating the partitions outside the database, but nothing was ever
// scheduled to do it, so the log piled into the DEFAULT partition. This
// maintainer is that schedule: an ensure-step at startup plus a slow tick, both
// idempotent and safe to run from every replica.
//
// It touches DDL only. The event log itself stays append-only — creating a
// partition moves no rows and rewrites no state.
//
// It stops the bleeding; it does not repair the backlog already in the DEFAULT
// partitions, which is an operational migration (detach default, create the
// missing months, re-insert with the explicit column list so event_seq is
// preserved, truncate, re-attach) run in a write-quiet window. Until that runs,
// every tick reports the months it cannot create.
package partition_maintainer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// defaultMonthsAhead is how far past the current month partitions are
	// pre-created. Three months means a service that stays down for a full
	// quarter still writes into a real partition on the way back up, and
	// each CREATE runs while the default partition is empty enough for its
	// mandatory scan to be cheap.
	defaultMonthsAhead = 3

	// DefaultTickInterval re-runs the ensure-step often enough to cross a
	// month boundary on a long-lived replica, rarely enough that the
	// existence probe is the only cost on a steady-state database.
	DefaultTickInterval = 6 * time.Hour
)

// defaultTables are the partitioned append-only event tables (migrations
// 00006 and 00007).
var defaultTables = []string{"knowledge_events", "knowledge_user_events"}

// Repository is the narrow surface the maintainer needs.
type Repository interface {
	EnsurePartitions(ctx context.Context, tableName string, startMonth time.Time, months int) ([]string, error)
}

// Config tunes the tables covered and the lookahead.
type Config struct {
	Tables      []string
	MonthsAhead int
	Clock       func() time.Time
}

// Maintainer ensures the upcoming monthly partitions exist.
type Maintainer struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
}

func New(repo Repository, logger *slog.Logger, cfg Config) *Maintainer {
	if logger == nil {
		logger = slog.Default()
	}
	if len(cfg.Tables) == 0 {
		cfg.Tables = defaultTables
	}
	if cfg.MonthsAhead <= 0 {
		cfg.MonthsAhead = defaultMonthsAhead
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Maintainer{repo: repo, logger: logger, cfg: cfg}
}

// RunOnce ensures the current month and the configured lookahead exist for
// every configured table. It is idempotent: on a database that is already
// ahead it issues only the existence probes.
//
// The current month is included deliberately. On a database whose backlog is
// still sitting in the default partition it cannot be created until an operator
// splits that partition, so every tick reports it — a standing, loud signal
// rather than a silence. The future months are created regardless.
//
// Repeating that report costs nothing on the hot table: the driver refuses the
// month from an indexed ACCESS SHARE range probe (ErrDefaultPartitionBacklog)
// instead of letting the CREATE discover it by taking ACCESS EXCLUSIVE on
// knowledge_events and rescanning the default partition every six hours. The
// tick after the operator procedure runs, the probe comes back empty and the
// month is created — no restart, no per-process memory to clear.
//
// A failing table does not skip the others; all failures are joined and
// returned for the caller to log.
func (m *Maintainer) RunOnce(ctx context.Context) error {
	now := m.cfg.Clock().UTC()
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	var errs []error
	for _, table := range m.cfg.Tables {
		created, err := m.repo.EnsurePartitions(ctx, table, startMonth, m.cfg.MonthsAhead+1)
		if len(created) > 0 {
			m.logger.Info("partition_maintainer created partitions",
				"table", table, "partitions", created)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("partition_maintainer %s: %w", table, err))
		}
	}
	return errors.Join(errs...)
}
