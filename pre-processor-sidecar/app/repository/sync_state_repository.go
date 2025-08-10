// ABOUTME: PostgreSQL implementation of SyncStateRepository interface
// ABOUTME: Manages continuation tokens for paginated Inoreader API requests

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"pre-processor-sidecar/models"
	"github.com/google/uuid"
)

// PostgreSQLSyncStateRepository implements SyncStateRepository using PostgreSQL
type PostgreSQLSyncStateRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewPostgreSQLSyncStateRepository creates a new PostgreSQL sync state repository
func NewPostgreSQLSyncStateRepository(db *sql.DB, logger *slog.Logger) SyncStateRepository {
	return &PostgreSQLSyncStateRepository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new sync state record
func (r *PostgreSQLSyncStateRepository) Create(ctx context.Context, syncState *models.SyncState) error {
	query := `
		INSERT INTO sync_state (id, stream_id, continuation_token, last_sync, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	now := time.Now()
	_, err := r.db.ExecContext(ctx, query,
		syncState.ID,
		syncState.StreamID,
		syncState.ContinuationToken,
		syncState.LastSync,
		now,
	)

	if err != nil {
		r.logger.Error("Failed to create sync state",
			"stream_id", syncState.StreamID,
			"error", err)
		return fmt.Errorf("failed to create sync state: %w", err)
	}

	r.logger.Debug("Created sync state successfully",
		"stream_id", syncState.StreamID,
		"continuation_token", syncState.ContinuationToken != "")

	return nil
}

// FindByStreamID finds a sync state by stream ID
func (r *PostgreSQLSyncStateRepository) FindByStreamID(ctx context.Context, streamID string) (*models.SyncState, error) {
	query := `
		SELECT id, stream_id, continuation_token, last_sync
		FROM sync_state
		WHERE stream_id = $1`

	var syncState models.SyncState
	err := r.db.QueryRowContext(ctx, query, streamID).Scan(
		&syncState.ID,
		&syncState.StreamID,
		&syncState.ContinuationToken,
		&syncState.LastSync,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("sync state not found for stream_id: %s", streamID)
		}
		return nil, fmt.Errorf("failed to find sync state by stream_id: %w", err)
	}

	return &syncState, nil
}

// FindByID finds a sync state by its UUID
func (r *PostgreSQLSyncStateRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.SyncState, error) {
	query := `
		SELECT id, stream_id, continuation_token, last_sync
		FROM sync_state
		WHERE id = $1`

	var syncState models.SyncState
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&syncState.ID,
		&syncState.StreamID,
		&syncState.ContinuationToken,
		&syncState.LastSync,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("sync state not found with id: %s", id.String())
		}
		return nil, fmt.Errorf("failed to find sync state by id: %w", err)
	}

	return &syncState, nil
}

// GetAll retrieves all sync states
func (r *PostgreSQLSyncStateRepository) GetAll(ctx context.Context) ([]*models.SyncState, error) {
	query := `
		SELECT id, stream_id, continuation_token, last_sync
		FROM sync_state
		ORDER BY last_sync DESC`

	return r.querySyncStates(ctx, query)
}

// GetStaleStates retrieves sync states that are older than specified time
func (r *PostgreSQLSyncStateRepository) GetStaleStates(ctx context.Context, olderThan time.Time) ([]*models.SyncState, error) {
	query := `
		SELECT id, stream_id, continuation_token, last_sync
		FROM sync_state
		WHERE last_sync < $1
		ORDER BY last_sync ASC`

	return r.querySyncStates(ctx, query, olderThan)
}

// Update updates an existing sync state
func (r *PostgreSQLSyncStateRepository) Update(ctx context.Context, syncState *models.SyncState) error {
	query := `
		UPDATE sync_state
		SET continuation_token = $2, last_sync = $3
		WHERE stream_id = $1`

	result, err := r.db.ExecContext(ctx, query,
		syncState.StreamID,
		syncState.ContinuationToken,
		syncState.LastSync,
	)

	if err != nil {
		r.logger.Error("Failed to update sync state",
			"stream_id", syncState.StreamID,
			"error", err)
		return fmt.Errorf("failed to update sync state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sync state not found for update: %s", syncState.StreamID)
	}

	r.logger.Debug("Updated sync state successfully",
		"stream_id", syncState.StreamID,
		"continuation_token", syncState.ContinuationToken != "")

	return nil
}

// UpdateContinuationToken updates only the continuation token and sync time
func (r *PostgreSQLSyncStateRepository) UpdateContinuationToken(ctx context.Context, streamID, token string) error {
	query := `
		UPDATE sync_state
		SET continuation_token = $2, last_sync = $3
		WHERE stream_id = $1`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query, streamID, token, now)

	if err != nil {
		r.logger.Error("Failed to update continuation token",
			"stream_id", streamID,
			"error", err)
		return fmt.Errorf("failed to update continuation token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sync state not found for token update: %s", streamID)
	}

	r.logger.Debug("Updated continuation token successfully",
		"stream_id", streamID,
		"has_token", token != "")

	return nil
}

// Delete deletes a sync state by ID
func (r *PostgreSQLSyncStateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM sync_state WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete sync state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sync state not found for deletion: %s", id.String())
	}

	r.logger.Debug("Deleted sync state successfully", "id", id.String())
	return nil
}

// DeleteByStreamID deletes a sync state by stream ID
func (r *PostgreSQLSyncStateRepository) DeleteByStreamID(ctx context.Context, streamID string) error {
	query := `DELETE FROM sync_state WHERE stream_id = $1`

	result, err := r.db.ExecContext(ctx, query, streamID)
	if err != nil {
		return fmt.Errorf("failed to delete sync state by stream_id: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("sync state not found for deletion: %s", streamID)
	}

	r.logger.Debug("Deleted sync state by stream_id successfully", "stream_id", streamID)
	return nil
}

// DeleteStale deletes sync states older than specified time
func (r *PostgreSQLSyncStateRepository) DeleteStale(ctx context.Context, olderThan time.Time) (int, error) {
	query := `DELETE FROM sync_state WHERE last_sync < $1`

	result, err := r.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete stale sync states: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	deletedCount := int(rowsAffected)
	r.logger.Info("Deleted stale sync states",
		"count", deletedCount,
		"older_than", olderThan)

	return deletedCount, nil
}

// CleanupStale removes sync states older than specified retention days
func (r *PostgreSQLSyncStateRepository) CleanupStale(ctx context.Context, retentionDays int) (int, error) {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	r.logger.Info("Starting sync state cleanup",
		"retention_days", retentionDays,
		"cutoff_time", cutoffTime)

	deletedCount, err := r.DeleteStale(ctx, cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup stale sync states: %w", err)
	}

	r.logger.Info("Sync state cleanup completed",
		"deleted_count", deletedCount,
		"retention_days", retentionDays)

	return deletedCount, nil
}

// querySyncStates is a helper method to execute queries that return multiple sync states
func (r *PostgreSQLSyncStateRepository) querySyncStates(ctx context.Context, query string, args ...interface{}) ([]*models.SyncState, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sync states: %w", err)
	}
	defer rows.Close()

	var syncStates []*models.SyncState
	for rows.Next() {
		syncState := &models.SyncState{}
		err := rows.Scan(
			&syncState.ID,
			&syncState.StreamID,
			&syncState.ContinuationToken,
			&syncState.LastSync,
		)
		if err != nil {
			r.logger.Error("Failed to scan sync state row", "error", err)
			continue
		}

		syncStates = append(syncStates, syncState)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return syncStates, nil
}