package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type upsertRecallCandidateMutation struct {
	UserID            uuid.UUID
	TenantID          *uuid.UUID
	ItemKey           string
	RecallScore       float64
	ReasonJSON        string
	NextSuggestAt     *time.Time
	FirstEligibleAt   *time.Time
	UpdatedAt         time.Time
	ProjectionVersion int
}

func parseUpsertRecallCandidateMutation(payload json.RawMessage) (upsertRecallCandidateMutation, error) {
	var candidate struct {
		UserID            uuid.UUID      `json:"user_id"`
		TenantID          string         `json:"tenant_id"`
		ItemKey           string         `json:"item_key"`
		RecallScore       float64        `json:"recall_score"`
		Reasons           []RecallReason `json:"reasons"`
		NextSuggestAt     *time.Time     `json:"next_suggest_at"`
		FirstEligibleAt   *time.Time     `json:"first_eligible_at"`
		UpdatedAt         time.Time      `json:"updated_at"`
		ProjectionVersion int            `json:"projection_version"`
	}
	if err := json.Unmarshal(payload, &candidate); err != nil {
		return upsertRecallCandidateMutation{}, fmt.Errorf("UpsertRecallCandidate: unmarshal: %w", err)
	}

	reasonJSON, err := json.Marshal(candidate.Reasons)
	if err != nil {
		return upsertRecallCandidateMutation{}, fmt.Errorf("UpsertRecallCandidate: marshal reasons: %w", err)
	}

	var tenantID *uuid.UUID
	if candidate.TenantID != "" {
		tID, err := uuid.Parse(candidate.TenantID)
		if err != nil {
			return upsertRecallCandidateMutation{}, fmt.Errorf("UpsertRecallCandidate: parse tenant_id: %w", err)
		}
		tenantID = &tID
	}

	return upsertRecallCandidateMutation{
		UserID:            candidate.UserID,
		TenantID:          tenantID,
		ItemKey:           candidate.ItemKey,
		RecallScore:       candidate.RecallScore,
		ReasonJSON:        string(reasonJSON),
		NextSuggestAt:     candidate.NextSuggestAt,
		FirstEligibleAt:   candidate.FirstEligibleAt,
		UpdatedAt:         candidate.UpdatedAt,
		ProjectionVersion: candidate.ProjectionVersion,
	}, nil
}

func buildUpsertRecallCandidateArgs(m upsertRecallCandidateMutation) []any {
	args := []any{
		m.UserID, m.ItemKey, m.RecallScore, m.ReasonJSON,
		m.NextSuggestAt, m.FirstEligibleAt, m.UpdatedAt, m.ProjectionVersion,
	}
	if m.TenantID != nil {
		args = append(args, *m.TenantID)
	}
	return args
}

const upsertRecallCandidateQuery = `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version`

const upsertRecallCandidateWithHomeAssociationQuery = `INSERT INTO recall_candidate_view
		(user_id, item_key, recall_score, reason_json, next_suggest_at, first_eligible_at, updated_at, projection_version)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8
		WHERE EXISTS (
			SELECT 1 FROM knowledge_home_items khi
			WHERE khi.user_id = $1
			  AND khi.item_key = $2
			  AND khi.projection_version = $8
			  AND khi.tenant_id = $9
		)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		  recall_score = EXCLUDED.recall_score,
		  reason_json = EXCLUDED.reason_json,
		  next_suggest_at = EXCLUDED.next_suggest_at,
		  updated_at = EXCLUDED.updated_at,
		  projection_version = EXCLUDED.projection_version
		WHERE EXISTS (
			SELECT 1 FROM knowledge_home_items khi
			WHERE khi.user_id = recall_candidate_view.user_id
			  AND khi.item_key = recall_candidate_view.item_key
			  AND khi.projection_version = recall_candidate_view.projection_version
			  AND khi.tenant_id = $9
		)`

// UpsertRecallCandidate inserts or updates a recall candidate.
func (r *Repository) UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	m, err := parseUpsertRecallCandidateMutation(payload)
	if err != nil {
		return err
	}
	query := upsertRecallCandidateQuery
	if m.TenantID != nil {
		query = upsertRecallCandidateWithHomeAssociationQuery
	}
	commandTag, err := r.pool.Exec(ctx, query, buildUpsertRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	if m.TenantID != nil && commandTag.RowsAffected() == 0 {
		return ErrRecallHomeAssociationDenied
	}
	return nil
}

type snoozeRecallCandidateMutation struct {
	UserID     uuid.UUID
	TenantID   *uuid.UUID
	ItemKey    string
	Until      time.Time
	OccurredAt time.Time
}

func parseSnoozeRecallCandidateMutation(payload json.RawMessage) (snoozeRecallCandidateMutation, error) {
	var params struct {
		UserID     string `json:"user_id"`
		TenantID   string `json:"tenant_id"`
		ItemKey    string `json:"item_key"`
		Until      string `json:"until"`
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: parse user_id: %w", err)
	}
	var tenantID *uuid.UUID
	if params.TenantID != "" {
		tID, err := uuid.Parse(params.TenantID)
		if err != nil {
			return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: parse tenant_id: %w", err)
		}
		tenantID = &tID
	}
	until, err := time.Parse(time.RFC3339Nano, params.Until)
	if err != nil {
		return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: parse until: %w", err)
	}
	if params.OccurredAt == "" {
		return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: occurred_at is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, params.OccurredAt)
	if err != nil {
		return snoozeRecallCandidateMutation{}, fmt.Errorf("SnoozeRecallCandidate: parse occurred_at: %w", err)
	}
	return snoozeRecallCandidateMutation{
		UserID:     userID,
		TenantID:   tenantID,
		ItemKey:    params.ItemKey,
		Until:      until,
		OccurredAt: occurredAt,
	}, nil
}

func buildSnoozeRecallCandidateArgs(m snoozeRecallCandidateMutation) []any {
	args := []any{m.Until, m.OccurredAt, m.UserID, m.ItemKey}
	if m.TenantID != nil {
		args = append(args, *m.TenantID)
	}
	return args
}

const snoozeRecallCandidateQuery = `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = $2
		WHERE user_id = $3 AND item_key = $4`

const snoozeRecallCandidateWithHomeAssociationQuery = `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = $2
		WHERE user_id = $3 AND item_key = $4
		  AND EXISTS (
		    SELECT 1 FROM knowledge_home_items khi
		    WHERE khi.user_id = recall_candidate_view.user_id
		      AND khi.item_key = recall_candidate_view.item_key
		      AND khi.projection_version = recall_candidate_view.projection_version
		      AND khi.tenant_id = $5
		  )`

// SnoozeRecallCandidate snoozes a recall candidate until the given time.
// updated_at is written from the caller-supplied occurred_at rather than SQL
// now() — recall_candidate_view is a disposable projection (immutable-design-
// guard: Event-time purity), and now() would make ApplyRecallMutation's
// resend/replay non-deterministic.
func (r *Repository) SnoozeRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	m, err := parseSnoozeRecallCandidateMutation(payload)
	if err != nil {
		return err
	}
	query := snoozeRecallCandidateQuery
	if m.TenantID != nil {
		query = snoozeRecallCandidateWithHomeAssociationQuery
	}
	commandTag, err := r.pool.Exec(ctx, query, buildSnoozeRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	if m.TenantID != nil && commandTag.RowsAffected() == 0 {
		return ErrRecallHomeAssociationDenied
	}
	return nil
}

type dismissRecallCandidateMutation struct {
	UserID     uuid.UUID
	TenantID   *uuid.UUID
	ItemKey    string
	OccurredAt time.Time
}

func parseDismissRecallCandidateMutation(payload json.RawMessage) (dismissRecallCandidateMutation, error) {
	var params struct {
		UserID     string `json:"user_id"`
		TenantID   string `json:"tenant_id"`
		ItemKey    string `json:"item_key"`
		OccurredAt string `json:"occurred_at"`
	}
	if err := json.Unmarshal(payload, &params); err != nil {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: unmarshal: %w", err)
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: parse user_id: %w", err)
	}
	var tenantID *uuid.UUID
	if params.TenantID != "" {
		tID, err := uuid.Parse(params.TenantID)
		if err != nil {
			return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: parse tenant_id: %w", err)
		}
		tenantID = &tID
	}
	if params.OccurredAt == "" {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: occurred_at is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, params.OccurredAt)
	if err != nil {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: parse occurred_at: %w", err)
	}
	return dismissRecallCandidateMutation{
		UserID:     userID,
		TenantID:   tenantID,
		ItemKey:    params.ItemKey,
		OccurredAt: occurredAt,
	}, nil
}

func buildDismissRecallCandidateArgs(m dismissRecallCandidateMutation) []any {
	args := []any{m.OccurredAt, m.UserID, m.ItemKey}
	if m.TenantID != nil {
		args = append(args, *m.TenantID)
	}
	return args
}

const dismissRecallCandidateQuery = `UPDATE recall_candidate_view SET dismissed_at = $1, updated_at = $1
		WHERE user_id = $2 AND item_key = $3`

const dismissRecallCandidateWithHomeAssociationQuery = `UPDATE recall_candidate_view SET dismissed_at = $1, updated_at = $1
		WHERE user_id = $2 AND item_key = $3
		  AND EXISTS (
		    SELECT 1 FROM knowledge_home_items khi
		    WHERE khi.user_id = recall_candidate_view.user_id
		      AND khi.item_key = recall_candidate_view.item_key
		      AND khi.projection_version = recall_candidate_view.projection_version
		      AND khi.tenant_id = $4
		  )`

// DismissRecallCandidate soft-deletes a recall candidate by setting dismissed_at.
// The candidate remains in the table so the projector's UPSERT preserves the dismissal.
// After a 30-day cooldown, the projector may clear dismissed_at to allow re-surfacing.
// dismissed_at/updated_at are written from the caller-supplied occurred_at
// rather than SQL now() for the same reproject-determinism reason as
// SnoozeRecallCandidate above.
func (r *Repository) DismissRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	m, err := parseDismissRecallCandidateMutation(payload)
	if err != nil {
		return err
	}
	query := dismissRecallCandidateQuery
	if m.TenantID != nil {
		query = dismissRecallCandidateWithHomeAssociationQuery
	}
	commandTag, err := r.pool.Exec(ctx, query, buildDismissRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	if m.TenantID != nil && commandTag.RowsAffected() == 0 {
		return ErrRecallHomeAssociationDenied
	}
	return nil
}
