package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type augurConversationRepository struct {
	pool *pgxpool.Pool
}

// NewAugurConversationRepository returns a postgres-backed repository for
// persisted Ask Augur chats. All writes are INSERTs; updates happen only via
// new rows on augur_messages and deletes cascade from augur_conversations.
func NewAugurConversationRepository(pool *pgxpool.Pool) domain.AugurConversationRepository {
	return &augurConversationRepository{pool: pool}
}

func (r *augurConversationRepository) CreateConversation(ctx context.Context, conv *domain.AugurConversation) error {
	if conv == nil {
		return errors.New("augur repo: nil conversation")
	}
	if conv.Title == "" {
		return errors.New("augur repo: title must be set at creation")
	}
	const q = `
		INSERT INTO augur_conversations (id, user_id, title, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.pool.Exec(ctx, q, conv.ID, conv.UserID, conv.Title, conv.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert augur_conversation: %w", err)
	}
	return nil
}

func (r *augurConversationRepository) GetConversation(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.AugurConversation, error) {
	const q = `
		SELECT id, user_id, title, created_at
		FROM augur_conversations
		WHERE id = $1 AND user_id = $2
	`
	var c domain.AugurConversation
	err := r.pool.QueryRow(ctx, q, id, userID).
		Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select augur_conversation: %w", err)
	}
	return &c, nil
}

func (r *augurConversationRepository) AppendMessage(ctx context.Context, msg *domain.AugurMessage) error {
	if msg == nil {
		return errors.New("augur repo: nil message")
	}
	citations := msg.Citations
	if citations == nil {
		citations = []domain.AugurCitation{}
	}
	payload, err := json.Marshal(citations)
	if err != nil {
		return fmt.Errorf("marshal citations: %w", err)
	}
	const q = `
		INSERT INTO augur_messages (id, conversation_id, role, content, citations, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
	`
	_, err = r.pool.Exec(ctx, q, msg.ID, msg.ConversationID, msg.Role, msg.Content, string(payload), msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert augur_message: %w", err)
	}
	return nil
}

func (r *augurConversationRepository) ListMessages(ctx context.Context, conversationID uuid.UUID) ([]domain.AugurMessage, error) {
	const q = `
		SELECT id, conversation_id, role, content, citations, created_at
		FROM augur_messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC, id ASC
	`
	rows, err := r.pool.Query(ctx, q, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query augur_messages: %w", err)
	}
	defer rows.Close()

	var out []domain.AugurMessage
	for rows.Next() {
		var m domain.AugurMessage
		var citationsRaw []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &citationsRaw, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan augur_message: %w", err)
		}
		if len(citationsRaw) > 0 {
			if err := json.Unmarshal(citationsRaw, &m.Citations); err != nil {
				return nil, fmt.Errorf("unmarshal citations: %w", err)
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter augur_messages: %w", err)
	}
	return out, nil
}

func (r *augurConversationRepository) ListSummaries(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
	afterActivity *time.Time,
	afterID *uuid.UUID,
) ([]domain.AugurConversationSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Keyset pagination over (last_activity_at DESC, id DESC). For the first
	// page afterActivity/afterID are nil, so the bound is skipped.
	const base = `
		SELECT id, user_id, title, created_at,
		       last_activity_at, message_count,
		       COALESCE(last_message_preview, '')
		FROM augur_conversation_index
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	q := base
	if afterActivity != nil && afterID != nil {
		q += ` AND (last_activity_at, id) < ($2, $3)`
		args = append(args, *afterActivity, *afterID)
	}
	q += ` ORDER BY last_activity_at DESC, id DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query augur_conversation_index: %w", err)
	}
	defer rows.Close()

	var out []domain.AugurConversationSummary
	for rows.Next() {
		var s domain.AugurConversationSummary
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.Title, &s.CreatedAt,
			&s.LastActivityAt, &s.MessageCount, &s.LastMessagePreview,
		); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter summaries: %w", err)
	}
	return out, nil
}

func (r *augurConversationRepository) DeleteConversation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	const q = `
		DELETE FROM augur_conversations
		WHERE id = $1 AND user_id = $2
	`
	_, err := r.pool.Exec(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("delete augur_conversation: %w", err)
	}
	return nil
}
