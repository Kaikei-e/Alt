package alt_db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// GetProjectionCheckpoint returns the last_event_seq for a projector.
// Returns 0 if no checkpoint exists.
func (r *AltDBRepository) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetProjectionCheckpoint")
	defer span.End()

	query := `SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`

	var lastSeq int64
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&lastSeq)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("GetProjectionCheckpoint: %w", err)
	}

	return lastSeq, nil
}

// UpdateProjectionCheckpoint upserts the checkpoint for a projector.
func (r *AltDBRepository) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpdateProjectionCheckpoint")
	defer span.End()

	query := `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (projector_name) DO UPDATE SET
		 last_event_seq = EXCLUDED.last_event_seq,
		 updated_at = EXCLUDED.updated_at`

	_, err := r.pool.Exec(ctx, query, projectorName, lastSeq, time.Now())
	if err != nil {
		return fmt.Errorf("UpdateProjectionCheckpoint: %w", err)
	}

	return nil
}
