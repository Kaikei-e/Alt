package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RetentionPolicy defines the hot/warm/cold retention windows per entity type.
type RetentionPolicy struct {
	SystemEventsHot        time.Duration // knowledge_events (system): hot window in PG
	UserEventsHot          time.Duration // knowledge_events (user) + knowledge_user_events
	SupersededVersionsHot  time.Duration // superseded summary/tag versions
	WarmWindow             time.Duration // warm = detached partition, still in PG
}

// DefaultRetentionPolicy returns the standard retention policy.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		SystemEventsHot:       30 * 24 * time.Hour,  // 30 days
		UserEventsHot:         7 * 24 * time.Hour,   // 7 days
		SupersededVersionsHot: 30 * 24 * time.Hour,  // 30 days
		WarmWindow:            60 * 24 * time.Hour,   // 60 days (warm after hot)
	}
}

// PartitionInfo describes a table partition.
type PartitionInfo struct {
	Name       string
	RangeStart time.Time
	RangeEnd   time.Time
	RowCount   int64
	SizeBytes  int64
}

// RetentionLogEntry records a retention operation.
type RetentionLogEntry struct {
	LogID           uuid.UUID
	RunAt           time.Time
	Action          string // export, verify, aggregate, detach, drop
	TargetTable     string
	TargetPartition string
	RowsAffected    int64
	ArchivePath     string
	Checksum        string
	DryRun          bool
	Status          string // success, failed
	ErrorMessage    string
}

// PartitionsEligibleForArchive returns partitions whose data is entirely
// outside the hot window for the given table.
func (p RetentionPolicy) PartitionsEligibleForArchive(tableName string, partitions []PartitionInfo, now time.Time) []PartitionInfo {
	var hotWindow time.Duration
	switch tableName {
	case "knowledge_user_events":
		hotWindow = p.UserEventsHot
	default:
		hotWindow = p.SystemEventsHot
	}

	cutoff := now.Add(-hotWindow)
	var eligible []PartitionInfo
	for _, part := range partitions {
		// A partition is eligible if its entire range is before the cutoff.
		// For monthly partitions, RangeEnd is the first day of the next month.
		rangeEnd := part.RangeEnd
		if rangeEnd.IsZero() {
			// Infer from name: partition covers 1 month starting at RangeStart
			rangeEnd = part.RangeStart.AddDate(0, 1, 0)
		}
		if rangeEnd.Before(cutoff) || rangeEnd.Equal(cutoff) {
			eligible = append(eligible, part)
		}
	}
	return eligible
}

// ListPartitions returns the partitions of a partitioned table.
func (r *Repository) ListPartitions(ctx context.Context, tableName string) ([]PartitionInfo, error) {
	query := `SELECT
		child.relname AS partition_name,
		pg_get_expr(child.relpartbound, child.oid) AS bound_expr,
		pg_total_relation_size(child.oid) AS size_bytes
	FROM pg_inherits
	JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
	JOIN pg_class child ON pg_inherits.inhrelid = child.oid
	WHERE parent.relname = $1
	ORDER BY child.relname`

	rows, err := r.pool.Query(ctx, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("ListPartitions: %w", err)
	}
	defer rows.Close()

	var partitions []PartitionInfo
	for rows.Next() {
		var p PartitionInfo
		var boundExpr string
		if err := rows.Scan(&p.Name, &boundExpr, &p.SizeBytes); err != nil {
			return nil, fmt.Errorf("ListPartitions scan: %w", err)
		}
		p.RangeStart, p.RangeEnd = parseBoundExpr(boundExpr)
		partitions = append(partitions, p)
	}
	return partitions, nil
}

// InsertRetentionLog records a retention operation.
func (r *Repository) InsertRetentionLog(ctx context.Context, entry RetentionLogEntry) error {
	query := `INSERT INTO knowledge_retention_log
		(log_id, run_at, action, target_table, target_partition,
		 rows_affected, archive_path, checksum, dry_run, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.pool.Exec(ctx, query,
		entry.LogID, entry.RunAt, entry.Action, entry.TargetTable, entry.TargetPartition,
		entry.RowsAffected, entry.ArchivePath, entry.Checksum, entry.DryRun, entry.Status, entry.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("InsertRetentionLog: %w", err)
	}
	return nil
}

// ListRetentionLogs returns recent retention log entries.
func (r *Repository) ListRetentionLogs(ctx context.Context, limit int) ([]RetentionLogEntry, error) {
	query := `SELECT log_id, run_at, action, target_table, COALESCE(target_partition, ''),
		rows_affected, COALESCE(archive_path, ''), COALESCE(checksum, ''),
		dry_run, status, COALESCE(error_message, '')
		FROM knowledge_retention_log
		ORDER BY run_at DESC LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("ListRetentionLogs: %w", err)
	}
	defer rows.Close()

	var entries []RetentionLogEntry
	for rows.Next() {
		var e RetentionLogEntry
		if err := rows.Scan(
			&e.LogID, &e.RunAt, &e.Action, &e.TargetTable, &e.TargetPartition,
			&e.RowsAffected, &e.ArchivePath, &e.Checksum, &e.DryRun, &e.Status, &e.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("ListRetentionLogs scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// parseBoundExpr extracts start/end timestamps from a PostgreSQL partition bound expression.
// Format: "FOR VALUES FROM ('2026-03-01 00:00:00+00') TO ('2026-04-01 00:00:00+00')"
func parseBoundExpr(expr string) (time.Time, time.Time) {
	var start, end time.Time
	// Simple parsing: extract the two timestamp strings
	// PostgreSQL format: FOR VALUES FROM ('YYYY-MM-DD HH:MM:SS+00') TO ('...')
	fmt.Sscanf(expr, "FOR VALUES FROM ('%d-%d-%d", &start)

	// More robust: use time.Parse on extracted portions
	const layout = "2006-01-02"
	startStr := extractQuotedDate(expr, "FROM")
	endStr := extractQuotedDate(expr, "TO")

	if startStr != "" {
		start, _ = time.Parse(layout, startStr[:10])
	}
	if endStr != "" {
		end, _ = time.Parse(layout, endStr[:10])
	}
	return start, end
}

// extractQuotedDate extracts the date from a bound expression after the given keyword.
func extractQuotedDate(expr, keyword string) string {
	idx := 0
	for i := 0; i <= len(expr)-len(keyword); i++ {
		if expr[i:i+len(keyword)] == keyword {
			idx = i + len(keyword)
			break
		}
	}
	if idx == 0 {
		return ""
	}
	// Find the opening quote
	for idx < len(expr) && expr[idx] != '\'' {
		idx++
	}
	if idx >= len(expr) {
		return ""
	}
	idx++ // skip opening quote
	// Find the closing quote
	end := idx
	for end < len(expr) && expr[end] != '\'' {
		end++
	}
	if end > idx {
		return expr[idx:end]
	}
	return ""
}
