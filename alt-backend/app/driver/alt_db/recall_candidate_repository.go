package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func (r *AltDBRepository) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetRecallCandidates")
	defer span.End()

	query := `SELECT user_id, item_key, recall_score, reason_json, next_suggest_at,
		first_eligible_at, snoozed_until, updated_at, projection_version
		FROM recall_candidate_view
		WHERE user_id = $1
		  AND (snoozed_until IS NULL OR snoozed_until < now())
		  AND next_suggest_at IS NOT NULL
		  AND next_suggest_at <= now()
		ORDER BY recall_score DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetRecallCandidates: %w", err)
	}
	defer rows.Close()

	var candidates []domain.RecallCandidate
	for rows.Next() {
		var c domain.RecallCandidate
		var reasonJSON []byte
		if err := rows.Scan(&c.UserID, &c.ItemKey, &c.RecallScore, &reasonJSON,
			&c.NextSuggestAt, &c.FirstEligibleAt, &c.SnoozedUntil, &c.UpdatedAt, &c.ProjectionVersion); err != nil {
			return nil, fmt.Errorf("GetRecallCandidates scan: %w", err)
		}
		_ = json.Unmarshal(reasonJSON, &c.Reasons)
		candidates = append(candidates, c)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(candidates)))
	return candidates, nil
}

func (r *AltDBRepository) UpsertRecallCandidate(ctx context.Context, candidate domain.RecallCandidate) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpsertRecallCandidate")
	defer span.End()

	reasonJSON, _ := json.Marshal(candidate.Reasons)

	query := `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`

	_, err := r.pool.Exec(ctx, query,
		candidate.UserID, candidate.ItemKey, candidate.RecallScore, reasonJSON,
		candidate.NextSuggestAt, candidate.FirstEligibleAt, candidate.UpdatedAt, candidate.ProjectionVersion,
	)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	return nil
}

func (r *AltDBRepository) SnoozeRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, until time.Time) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.SnoozeRecallCandidate")
	defer span.End()

	query := `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = now()
		WHERE user_id = $2 AND item_key = $3`
	_, err := r.pool.Exec(ctx, query, until, userID, itemKey)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	return nil
}

func (r *AltDBRepository) DismissRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.DismissRecallCandidate")
	defer span.End()

	query := `DELETE FROM recall_candidate_view WHERE user_id = $1 AND item_key = $2`
	_, err := r.pool.Exec(ctx, query, userID, itemKey)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	return nil
}
