package alt_db

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
)

// GetProjectionLag returns the current projection lag as a time.Duration.
// It queries the most recent updated_at from knowledge_projection_checkpoints
// and computes the difference from now().
// Returns -1 if no checkpoints exist (NULL result).
func (r *AltDBRepository) GetProjectionLag(ctx context.Context) (time.Duration, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetProjectionLag")
	defer span.End()

	query := `SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`

	var lagSeconds *float64
	err := r.pool.QueryRow(ctx, query).Scan(&lagSeconds)
	if err != nil {
		return 0, fmt.Errorf("GetProjectionLag: %w", err)
	}

	if lagSeconds == nil {
		return time.Duration(-1), nil
	}

	return time.Duration(*lagSeconds * float64(time.Second)), nil
}

// GetProjectionAge returns the age of the latest projection update as a time.Duration.
// This is functionally equivalent to GetProjectionLag but named for SLO clarity.
// Returns -1 if no checkpoints exist (NULL result).
func (r *AltDBRepository) GetProjectionAge(ctx context.Context) (time.Duration, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetProjectionAge")
	defer span.End()

	query := `SELECT EXTRACT(EPOCH FROM (now() - MAX(updated_at))) FROM knowledge_projection_checkpoints`

	var ageSeconds *float64
	err := r.pool.QueryRow(ctx, query).Scan(&ageSeconds)
	if err != nil {
		return 0, fmt.Errorf("GetProjectionAge: %w", err)
	}

	if ageSeconds == nil {
		return time.Duration(-1), nil
	}

	return time.Duration(*ageSeconds * float64(time.Second)), nil
}
