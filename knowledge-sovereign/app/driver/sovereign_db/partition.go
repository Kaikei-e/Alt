package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// PartitionSpec describes a partition to be created. Table/From/To carry the
// same facts the DDL text states, so the backlog probe can ask about the range
// without re-parsing the statement.
type PartitionSpec struct {
	Name  string
	DDL   string
	Table string
	From  time.Time
	To    time.Time
}

// ErrDefaultPartitionBacklog reports that a month cannot be partitioned yet
// because the DEFAULT partition still holds rows belonging in its range. The
// CREATE would fail; see EnsurePartitions for why the probe exists and the
// package docs of usecase/partition_maintainer for the operator procedure.
var ErrDefaultPartitionBacklog = errors.New("default partition still holds rows in this range; an operator must split it")

// partitionKeyColumn is the range key of both partitioned event tables:
// migrations 00006/00007 declare PARTITION BY RANGE (occurred_at).
const partitionKeyColumn = "occurred_at"

// setPartitionLockTimeout bounds how long the DDL below waits for the parent's
// ACCESS EXCLUSIVE lock. Nothing else in this service sets lock_timeout or
// statement_timeout, and DATABASE_URL carries no options, so without it an
// unattended maintenance tick can queue behind a long reader and hold up every
// knowledge_events INSERT behind it for as long as that reader runs. Five
// seconds: a lock that is not free almost immediately is one to retry on the
// next tick, not to wait for on the hot append path.
//
// Deliberately not statement_timeout: once the lock is held, the CREATE must be
// allowed to finish its mandatory default-partition scan, and cutting that off
// would mean the partition never gets created at all.
const setPartitionLockTimeout = "SET LOCAL lock_timeout = '5s'"

// GeneratePartitionDDL generates DDL statements for monthly partitions of the given table.
// It creates `count` partitions starting from `startMonth`.
func GeneratePartitionDDL(tableName string, startMonth time.Time, count int) []PartitionSpec {
	// Normalize to first of month in UTC
	current := time.Date(startMonth.Year(), startMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	specs := make([]PartitionSpec, 0, count)
	for i := 0; i < count; i++ {
		next := current.AddDate(0, 1, 0)
		name := fmt.Sprintf("%s_y%04dm%02d", tableName, current.Year(), current.Month())
		ddl := fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
			name,
			tableName,
			current.Format("2006-01-02"),
			next.Format("2006-01-02"),
		)
		specs = append(specs, PartitionSpec{
			Name:  name,
			DDL:   ddl,
			Table: tableName,
			From:  current,
			To:    next,
		})
		current = next
	}
	return specs
}

// EnsurePartitions creates the monthly partitions of tableName covering `months`
// months from startMonth, skipping the ones that already exist, and returns the
// names it actually created.
//
// Migrations 00006/00007 created a fixed six months each and nothing renewed
// them, so everything after 2026-05-01 fell into the DEFAULT partition. This is
// the renewal path: called ahead of time it creates the next months while the
// default partition is still small.
//
// Each month is created in its own transaction guarded by
// pg_advisory_xact_lock, so several replicas running this concurrently
// serialize instead of racing into a pg_type unique violation (CREATE TABLE IF
// NOT EXISTS is not itself race-free). The existence probe keeps the work at
// zero statements once the months are there: every CREATE against a table that
// has a DEFAULT partition forces a full scan of that default partition under
// ACCESS EXCLUSIVE, so re-issuing it every tick would be far from free.
//
// A month that fails does not abort the rest — while the default partition
// still holds the un-partitioned backlog, the current month cannot be created
// (its rows live in default, so it is refused with ErrDefaultPartitionBacklog
// before any lock is taken), but the future months that stop the bleeding
// still can. Failures are joined and returned for the caller to log.
func (r *Repository) EnsurePartitions(ctx context.Context, tableName string, startMonth time.Time, months int) ([]string, error) {
	created := make([]string, 0, months)
	var errs []error
	for _, spec := range GeneratePartitionDDL(tableName, startMonth, months) {
		ok, err := r.ensurePartition(ctx, spec)
		if err != nil {
			errs = append(errs, fmt.Errorf("EnsurePartitions %s: %w", spec.Name, err))
			continue
		}
		if ok {
			created = append(created, spec.Name)
		}
	}
	return created, errors.Join(errs...)
}

// ensurePartition creates one partition if it is absent, reporting whether it
// did. The advisory lock is keyed on the partition name so concurrent replicas
// contend only on the same month.
func (r *Repository) ensurePartition(ctx context.Context, spec PartitionSpec) (bool, error) {
	// Both checks below run outside the transaction on purpose. They cost a
	// catalog lookup and at most an ACCESS SHARE range probe, and keeping them
	// out of the DDL transaction means it never upgrades a lock it already
	// holds — two replicas ensuring different months would otherwise be able
	// to deadlock on the parent.
	exists, err := partitionExists(ctx, r.pool, spec.Name)
	if err != nil {
		return false, fmt.Errorf("existence check: %w", err)
	}
	if exists {
		return false, nil
	}

	blocked, err := r.defaultPartitionBlocks(ctx, spec)
	if err != nil {
		return false, fmt.Errorf("backlog probe: %w", err)
	}
	if blocked {
		return false, ErrDefaultPartitionBacklog
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	if _, err := tx.Exec(ctx, setPartitionLockTimeout); err != nil {
		return false, fmt.Errorf("lock timeout: %w", err)
	}

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", spec.Name); err != nil {
		return false, fmt.Errorf("advisory lock: %w", err)
	}

	// Re-probe under the advisory lock: another replica may have created the
	// partition between our probe above and this transaction.
	exists, err = partitionExists(ctx, tx, spec.Name)
	if err != nil {
		return false, fmt.Errorf("existence check: %w", err)
	}
	if exists {
		return false, nil
	}

	if _, err := tx.Exec(ctx, spec.DDL); err != nil {
		return false, fmt.Errorf("create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// rowQuerier is the QueryRow surface shared by the pool and a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// partitionExists reports whether the named relation is in the catalog.
// to_regclass takes no lock on the table it resolves, which is what keeps the
// steady state (every month already created) free.
func partitionExists(ctx context.Context, q rowQuerier, name string) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// defaultPartitionBlocks reports whether the parent table already holds a row
// inside spec's range, which is exactly the condition that makes the CREATE
// fail ("updated partition constraint for default partition would be violated
// by some row").
//
// Asking first is what keeps the maintainer off the hot append path. Letting
// the CREATE discover the answer costs ACCESS EXCLUSIVE on the parent plus a
// scan of the default partition, on every tick for as long as the backlog
// exists. Neither obvious alternative fixes that: setPartitionLockTimeout
// bounds only the wait *for* the lock, not the scan performed while holding it,
// and remembering the failure for the process lifetime would keep skipping the
// month after an operator had fixed the backlog, until the next restart. This
// probe is one indexed range lookup under ACCESS SHARE, blocks no append, and
// self-heals on the next tick.
//
// Querying the parent rather than <table>_default is deliberate: no partition
// covers this range yet, so a row inside it can only be in the default
// partition, and going through the parent needs no assumption about how the
// default partition is named.
// The bounds are passed as the same date literals the DDL carries, cast the
// same way, so probe and partition bound cannot disagree about where the month
// starts on a server whose TimeZone is not UTC.
func (r *Repository) defaultPartitionBlocks(ctx context.Context, spec PartitionSpec) (bool, error) {
	query := fmt.Sprintf(
		"SELECT EXISTS (SELECT 1 FROM %s WHERE %s >= $1::timestamptz AND %s < $2::timestamptz)",
		spec.Table, partitionKeyColumn, partitionKeyColumn)

	var found bool
	err := r.pool.QueryRow(ctx, query,
		spec.From.Format("2006-01-02"),
		spec.To.Format("2006-01-02"),
	).Scan(&found)
	if err != nil {
		return false, err
	}
	return found, nil
}
