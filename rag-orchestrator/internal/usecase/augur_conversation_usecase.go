package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// AugurConversationUsecase persists Ask Augur chat turns and exposes history
// reads. The write path is strictly append-only: new conversations and turns
// are INSERTed, never updated. Deletion is destructive (row + cascade).
type AugurConversationUsecase interface {
	// EnsureConversation returns the conversation row for (userID, conversationID).
	// If conversationID is the zero UUID, a new conversation is minted with a
	// title derived from firstUserMessage.
	EnsureConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, firstUserMessage string) (*domain.AugurConversation, error)

	AppendUserTurn(ctx context.Context, conversationID uuid.UUID, content string) error
	AppendAssistantTurn(ctx context.Context, conversationID uuid.UUID, content string, citations []domain.AugurCitation) error

	ListConversations(ctx context.Context, userID uuid.UUID, limit int, afterActivity *time.Time, afterID *uuid.UUID) ([]domain.AugurConversationSummary, error)
	GetConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) (*domain.AugurConversation, []domain.AugurMessage, error)
	DeleteConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error
}

// augurConversationUsecase is the default implementation.
type augurConversationUsecase struct {
	repo  domain.AugurConversationRepository
	clock func() time.Time
}

// NewAugurConversationUsecase wires a repository into the usecase. clock may
// be nil to use time.Now; tests inject a fixed clock for determinism.
func NewAugurConversationUsecase(repo domain.AugurConversationRepository, clock func() time.Time) AugurConversationUsecase {
	if clock == nil {
		clock = time.Now
	}
	return &augurConversationUsecase{repo: repo, clock: clock}
}

// titleFromFirstMessage derives a stable, presentable title from the first
// user turn: trim, collapse whitespace, cap at 80 runes, append an ellipsis if
// truncated. Kept deterministic and logic-free so SQL never has to run it.
func titleFromFirstMessage(raw string) string {
	trimmed := strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
	if trimmed == "" {
		return "Untitled chat"
	}
	const maxRunes = 80
	if utf8.RuneCountInString(trimmed) <= maxRunes {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:maxRunes]) + "…"
}

func (u *augurConversationUsecase) EnsureConversation(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	firstUserMessage string,
) (*domain.AugurConversation, error) {
	if userID == uuid.Nil {
		return nil, errors.New("augur usecase: userID required")
	}
	if conversationID != uuid.Nil {
		conv, err := u.repo.GetConversation(ctx, conversationID, userID)
		if err != nil {
			return nil, fmt.Errorf("load conversation: %w", err)
		}
		if conv != nil {
			return conv, nil
		}
	}
	conv := &domain.AugurConversation{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     titleFromFirstMessage(firstUserMessage),
		CreatedAt: u.clock().UTC(),
	}
	if err := u.repo.CreateConversation(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

func (u *augurConversationUsecase) AppendUserTurn(ctx context.Context, conversationID uuid.UUID, content string) error {
	return u.appendTurn(ctx, conversationID, "user", content, nil)
}

func (u *augurConversationUsecase) AppendAssistantTurn(ctx context.Context, conversationID uuid.UUID, content string, citations []domain.AugurCitation) error {
	return u.appendTurn(ctx, conversationID, "assistant", content, citations)
}

func (u *augurConversationUsecase) appendTurn(
	ctx context.Context,
	conversationID uuid.UUID,
	role string,
	content string,
	citations []domain.AugurCitation,
) error {
	if conversationID == uuid.Nil {
		return errors.New("augur usecase: conversationID required")
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("augur usecase: empty message content")
	}
	msg := &domain.AugurMessage{
		ID:             uuid.New(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Citations:      citations,
		CreatedAt:      u.clock().UTC(),
	}
	if err := u.repo.AppendMessage(ctx, msg); err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	return nil
}

func (u *augurConversationUsecase) ListConversations(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
	afterActivity *time.Time,
	afterID *uuid.UUID,
) ([]domain.AugurConversationSummary, error) {
	if userID == uuid.Nil {
		return nil, errors.New("augur usecase: userID required")
	}
	return u.repo.ListSummaries(ctx, userID, limit, afterActivity, afterID)
}

func (u *augurConversationUsecase) GetConversation(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
) (*domain.AugurConversation, []domain.AugurMessage, error) {
	if userID == uuid.Nil || conversationID == uuid.Nil {
		return nil, nil, errors.New("augur usecase: userID and conversationID required")
	}
	conv, err := u.repo.GetConversation(ctx, conversationID, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("load conversation: %w", err)
	}
	if conv == nil {
		return nil, nil, nil
	}
	msgs, err := u.repo.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("load messages: %w", err)
	}
	return conv, msgs, nil
}

func (u *augurConversationUsecase) DeleteConversation(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID) error {
	if userID == uuid.Nil || conversationID == uuid.Nil {
		return errors.New("augur usecase: userID and conversationID required")
	}
	return u.repo.DeleteConversation(ctx, conversationID, userID)
}
