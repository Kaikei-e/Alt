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

	return upsertRecallCandidateMutation{
		UserID:            candidate.UserID,
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
	return []any{
		m.UserID, m.ItemKey, m.RecallScore, m.ReasonJSON,
		m.NextSuggestAt, m.FirstEligibleAt, m.UpdatedAt, m.ProjectionVersion,
	}
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

// UpsertRecallCandidate inserts or updates a recall candidate.
func (r *Repository) UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error {
	m, err := parseUpsertRecallCandidateMutation(payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, upsertRecallCandidateQuery, buildUpsertRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("UpsertRecallCandidate: %w", err)
	}
	return nil
}

type snoozeRecallCandidateMutation struct {
	UserID     uuid.UUID
	ItemKey    string
	Until      time.Time
	OccurredAt time.Time
}

func parseSnoozeRecallCandidateMutation(payload json.RawMessage) (snoozeRecallCandidateMutation, error) {
	var params struct {
		UserID     string `json:"user_id"`
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
		ItemKey:    params.ItemKey,
		Until:      until,
		OccurredAt: occurredAt,
	}, nil
}

func buildSnoozeRecallCandidateArgs(m snoozeRecallCandidateMutation) []any {
	return []any{m.Until, m.OccurredAt, m.UserID, m.ItemKey}
}

const snoozeRecallCandidateQuery = `UPDATE recall_candidate_view SET snoozed_until = $1, updated_at = $2
		WHERE user_id = $3 AND item_key = $4`

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
	_, err = r.pool.Exec(ctx, snoozeRecallCandidateQuery, buildSnoozeRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("SnoozeRecallCandidate: %w", err)
	}
	return nil
}

type dismissRecallCandidateMutation struct {
	UserID     uuid.UUID
	ItemKey    string
	OccurredAt time.Time
}

func parseDismissRecallCandidateMutation(payload json.RawMessage) (dismissRecallCandidateMutation, error) {
	var params struct {
		UserID     string `json:"user_id"`
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
	if params.OccurredAt == "" {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: occurred_at is required")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, params.OccurredAt)
	if err != nil {
		return dismissRecallCandidateMutation{}, fmt.Errorf("DismissRecallCandidate: parse occurred_at: %w", err)
	}
	return dismissRecallCandidateMutation{
		UserID:     userID,
		ItemKey:    params.ItemKey,
		OccurredAt: occurredAt,
	}, nil
}

func buildDismissRecallCandidateArgs(m dismissRecallCandidateMutation) []any {
	return []any{m.OccurredAt, m.UserID, m.ItemKey}
}

const dismissRecallCandidateQuery = `UPDATE recall_candidate_view SET dismissed_at = $1, updated_at = $1
		WHERE user_id = $2 AND item_key = $3`

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
	_, err = r.pool.Exec(ctx, dismissRecallCandidateQuery, buildDismissRecallCandidateArgs(m)...)
	if err != nil {
		return fmt.Errorf("DismissRecallCandidate: %w", err)
	}
	return nil
}
