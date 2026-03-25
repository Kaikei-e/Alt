package sovereign_db

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotMetadata represents a projection snapshot record.
type SnapshotMetadata struct {
	SnapshotID        uuid.UUID
	SnapshotType      string // "full"
	ProjectionVersion int
	ProjectorBuildRef string
	SchemaVersion     string
	SnapshotAt        time.Time
	EventSeqBoundary  int64
	SnapshotDataPath  string
	ItemsRowCount     int
	ItemsChecksum     string
	DigestRowCount    int
	DigestChecksum    string
	RecallRowCount    int
	RecallChecksum    string
	CreatedAt         time.Time
	Status            string // pending, valid, invalidated, archived
}

// Validate checks that all required fields are present.
func (s *SnapshotMetadata) Validate() error {
	if s.ProjectorBuildRef == "" {
		return fmt.Errorf("projector_build_ref is required")
	}
	if s.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if s.EventSeqBoundary <= 0 {
		return fmt.Errorf("event_seq_boundary must be > 0")
	}
	if s.SnapshotDataPath == "" {
		return fmt.Errorf("snapshot_data_path is required")
	}
	if s.ItemsChecksum == "" {
		return fmt.Errorf("items_checksum is required")
	}
	if s.DigestChecksum == "" {
		return fmt.Errorf("digest_checksum is required")
	}
	if s.RecallChecksum == "" {
		return fmt.Errorf("recall_checksum is required")
	}
	return nil
}

// IsCompatibleWith checks if this snapshot can be used for reproject
// with the given schema and projector versions.
func (s *SnapshotMetadata) IsCompatibleWith(schemaVersion, projectorBuildRef string) bool {
	return s.SchemaVersion == schemaVersion && s.ProjectorBuildRef == projectorBuildRef
}

// InsertSnapshot saves snapshot metadata to the database.
func (r *Repository) InsertSnapshot(ctx context.Context, s *SnapshotMetadata) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("InsertSnapshot: %w", err)
	}

	query := `INSERT INTO knowledge_projection_snapshots
		(snapshot_id, snapshot_type, projection_version, projector_build_ref,
		 schema_version, snapshot_at, event_seq_boundary, snapshot_data_path,
		 items_row_count, items_checksum, digest_row_count, digest_checksum,
		 recall_row_count, recall_checksum, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	_, err := r.pool.Exec(ctx, query,
		s.SnapshotID, s.SnapshotType, s.ProjectionVersion, s.ProjectorBuildRef,
		s.SchemaVersion, s.SnapshotAt, s.EventSeqBoundary, s.SnapshotDataPath,
		s.ItemsRowCount, s.ItemsChecksum, s.DigestRowCount, s.DigestChecksum,
		s.RecallRowCount, s.RecallChecksum, s.Status,
	)
	if err != nil {
		return fmt.Errorf("InsertSnapshot: %w", err)
	}
	return nil
}

// UpdateSnapshotStatus updates the status of a snapshot.
func (r *Repository) UpdateSnapshotStatus(ctx context.Context, snapshotID uuid.UUID, status string) error {
	query := `UPDATE knowledge_projection_snapshots SET status = $1 WHERE snapshot_id = $2`
	_, err := r.pool.Exec(ctx, query, status, snapshotID)
	if err != nil {
		return fmt.Errorf("UpdateSnapshotStatus: %w", err)
	}
	return nil
}

// GetLatestValidSnapshot returns the most recent valid snapshot.
func (r *Repository) GetLatestValidSnapshot(ctx context.Context) (*SnapshotMetadata, error) {
	query := `SELECT snapshot_id, snapshot_type, projection_version, projector_build_ref,
		schema_version, snapshot_at, event_seq_boundary, snapshot_data_path,
		items_row_count, items_checksum, digest_row_count, digest_checksum,
		recall_row_count, recall_checksum, created_at, status
		FROM knowledge_projection_snapshots
		WHERE status = 'valid'
		ORDER BY event_seq_boundary DESC
		LIMIT 1`

	var s SnapshotMetadata
	err := r.pool.QueryRow(ctx, query).Scan(
		&s.SnapshotID, &s.SnapshotType, &s.ProjectionVersion, &s.ProjectorBuildRef,
		&s.SchemaVersion, &s.SnapshotAt, &s.EventSeqBoundary, &s.SnapshotDataPath,
		&s.ItemsRowCount, &s.ItemsChecksum, &s.DigestRowCount, &s.DigestChecksum,
		&s.RecallRowCount, &s.RecallChecksum, &s.CreatedAt, &s.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("GetLatestValidSnapshot: %w", err)
	}
	return &s, nil
}

// ListSnapshots returns all snapshots ordered by creation time.
func (r *Repository) ListSnapshots(ctx context.Context, limit int) ([]SnapshotMetadata, error) {
	query := `SELECT snapshot_id, snapshot_type, projection_version, projector_build_ref,
		schema_version, snapshot_at, event_seq_boundary, snapshot_data_path,
		items_row_count, items_checksum, digest_row_count, digest_checksum,
		recall_row_count, recall_checksum, created_at, status
		FROM knowledge_projection_snapshots
		ORDER BY event_seq_boundary DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("ListSnapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []SnapshotMetadata
	for rows.Next() {
		var s SnapshotMetadata
		if err := rows.Scan(
			&s.SnapshotID, &s.SnapshotType, &s.ProjectionVersion, &s.ProjectorBuildRef,
			&s.SchemaVersion, &s.SnapshotAt, &s.EventSeqBoundary, &s.SnapshotDataPath,
			&s.ItemsRowCount, &s.ItemsChecksum, &s.DigestRowCount, &s.DigestChecksum,
			&s.RecallRowCount, &s.RecallChecksum, &s.CreatedAt, &s.Status,
		); err != nil {
			return nil, fmt.Errorf("ListSnapshots scan: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

// GetMaxEventSeq returns the maximum event_seq from knowledge_events.
func (r *Repository) GetMaxEventSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := r.pool.QueryRow(ctx, "SELECT COALESCE(MAX(event_seq), 0) FROM knowledge_events").Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("GetMaxEventSeq: %w", err)
	}
	return seq, nil
}

// GetTableRowCount returns the row count of the given table.
func (r *Repository) GetTableRowCount(ctx context.Context, tableName string) (int, error) {
	// Use reltuples for fast approximate count on large tables
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("GetTableRowCount(%s): %w", tableName, err)
	}
	return count, nil
}

// ExportTableToWriter exports the contents of a table as JSONL to the given writer.
// Returns the number of rows exported.
// Requires the underlying pool to be *pgxpool.Pool (not a mock).
func (r *Repository) ExportTableToWriter(ctx context.Context, tableName string, w io.Writer) (int64, error) {
	pool, ok := r.pool.(*pgxpool.Pool)
	if !ok {
		return 0, fmt.Errorf("ExportTableToWriter: pool must be *pgxpool.Pool for COPY support")
	}

	query := fmt.Sprintf("COPY (SELECT row_to_json(t) FROM %s t) TO STDOUT", tableName)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("ExportTableToWriter acquire conn: %w", err)
	}
	defer conn.Release()

	tag, err := conn.Conn().PgConn().CopyTo(ctx, w, query)
	if err != nil {
		return 0, fmt.Errorf("ExportTableToWriter COPY: %w", err)
	}
	return tag.RowsAffected(), nil
}

// TableStorageInfo represents storage statistics for a table.
type TableStorageInfo struct {
	TableName     string `json:"table_name"`
	RowCount      int64  `json:"row_count"`
	TotalSize     string `json:"total_size"`
	TotalBytes    int64  `json:"total_bytes"`
	IsPartitioned bool   `json:"is_partitioned"`
}

// GetStorageStats returns storage statistics for all knowledge tables.
func (r *Repository) GetStorageStats(ctx context.Context) ([]TableStorageInfo, error) {
	query := `SELECT
		c.relname AS table_name,
		COALESCE(s.n_live_tup, 0) AS row_count,
		pg_size_pretty(pg_total_relation_size(c.oid)) AS total_size,
		pg_total_relation_size(c.oid) AS total_bytes,
		c.relkind = 'p' AS is_partitioned
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
	WHERE n.nspname = 'public'
	  AND c.relkind IN ('r', 'p')
	  AND c.relname LIKE 'knowledge_%'
	ORDER BY pg_total_relation_size(c.oid) DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetStorageStats: %w", err)
	}
	defer rows.Close()

	var stats []TableStorageInfo
	for rows.Next() {
		var s TableStorageInfo
		if err := rows.Scan(&s.TableName, &s.RowCount, &s.TotalSize, &s.TotalBytes, &s.IsPartitioned); err != nil {
			return nil, fmt.Errorf("GetStorageStats scan: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// InvalidateSnapshotsAfterSchemaChange marks all valid snapshots as invalidated
// when schema or projector version changes.
func (r *Repository) InvalidateSnapshotsAfterSchemaChange(ctx context.Context, currentSchema, currentBuildRef string) (int64, error) {
	query := `UPDATE knowledge_projection_snapshots
		SET status = 'invalidated'
		WHERE status = 'valid'
		AND (schema_version != $1 OR projector_build_ref != $2)`

	tag, err := r.pool.Exec(ctx, query, currentSchema, currentBuildRef)
	if err != nil {
		return 0, fmt.Errorf("InvalidateSnapshotsAfterSchemaChange: %w", err)
	}
	return tag.RowsAffected(), nil
}
