package reading_theme_usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/shared/port/knowledge_event_port"
)

type RecordThemeOpenedUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewRecordThemeOpenedUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *RecordThemeOpenedUsecase {
	if appendPort == nil {
		panic("reading_theme_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_theme_usecase: now clock is required")
	}
	return &RecordThemeOpenedUsecase{
		appendPort: appendPort,
		now:        now,
	}
}

type themeOpenedPayload struct {
	ThemeID  string `json:"theme_id"`
	Version  int32  `json:"version"`
	OpenedAt string `json:"opened_at"`
}

func (u *RecordThemeOpenedUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	tenantID uuid.UUID,
	rawThemeID string,
	version int32,
	rawClientEventID string,
	openedAt time.Time,
) error {
	if userID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", domain.ErrInvalidThemeInput)
	}
	themeID, err := uuid.Parse(rawThemeID)
	if err != nil {
		return fmt.Errorf("%w: invalid theme_id", domain.ErrInvalidThemeInput)
	}
	if version < 1 {
		return fmt.Errorf("%w: version must be at least 1", domain.ErrInvalidThemeInput)
	}
	clientEventID, err := uuid.Parse(rawClientEventID)
	if err != nil {
		return fmt.Errorf("%w: invalid client_event_id", domain.ErrInvalidThemeInput)
	}
	if openedAt.IsZero() {
		return fmt.Errorf("%w: opened_at is required", domain.ErrInvalidThemeInput)
	}

	receivedAt := u.now().UTC()
	clampedOccurredAt, _ := domain.ClampReadingOccurredAt(openedAt, receivedAt)

	payloadBytes, err := json.Marshal(themeOpenedPayload{
		ThemeID:  themeID.String(),
		Version:  version,
		OpenedAt: clampedOccurredAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal reading.theme_opened.v1 payload: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedOccurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingThemeOpened,
		AggregateType: domain.AggregateReadingTheme,
		AggregateID:   themeID.String(),
		DedupeKey:     "reading.theme_opened:" + clientEventID.String(),
		Payload:       payloadBytes,
	}

	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return fmt.Errorf("record theme opened: %w", err)
	}
	return nil
}
