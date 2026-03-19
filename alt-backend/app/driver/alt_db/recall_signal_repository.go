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

func (r *AltDBRepository) AppendRecallSignal(ctx context.Context, signal domain.RecallSignal) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.AppendRecallSignal")
	defer span.End()

	payload := signal.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadJSON, _ := json.Marshal(payload)

	query := `INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query,
		signal.SignalID, signal.UserID, signal.ItemKey, signal.SignalType,
		signal.SignalStrength, signal.OccurredAt, string(payloadJSON),
	)
	if err != nil {
		return fmt.Errorf("AppendRecallSignal: %w", err)
	}
	return nil
}

func (r *AltDBRepository) ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]domain.RecallSignal, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListRecallSignalsByUser")
	defer span.End()

	since := time.Now().AddDate(0, 0, -sinceDays)
	query := `SELECT signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload
		FROM recall_signals
		WHERE user_id = $1 AND occurred_at >= $2
		ORDER BY occurred_at DESC`

	rows, err := r.pool.Query(ctx, query, userID, since)
	if err != nil {
		return nil, fmt.Errorf("ListRecallSignalsByUser: %w", err)
	}
	defer rows.Close()

	var signals []domain.RecallSignal
	for rows.Next() {
		var s domain.RecallSignal
		var payloadJSON []byte
		if err := rows.Scan(&s.SignalID, &s.UserID, &s.ItemKey, &s.SignalType, &s.SignalStrength, &s.OccurredAt, &payloadJSON); err != nil {
			return nil, fmt.Errorf("ListRecallSignalsByUser scan: %w", err)
		}
		_ = json.Unmarshal(payloadJSON, &s.Payload)
		signals = append(signals, s)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(signals)))
	return signals, nil
}
