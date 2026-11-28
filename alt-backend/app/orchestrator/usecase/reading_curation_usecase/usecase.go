package reading_curation_usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_curation_port"
	"alt/shared/port/knowledge_event_port"
)

const (
	defaultListLimit int32 = 100
	maxListLimit     int32 = 500
)

type SaveAnnotationInput struct {
	ArticleID     string
	Stance        domain.AnnotationStance
	Excerpt       string
	Note          string
	ClientEventID string
	CreatedAt     time.Time
}

type SaveArticleAnnotationUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewSaveArticleAnnotationUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *SaveArticleAnnotationUsecase {
	if appendPort == nil {
		panic("reading_curation_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_curation_usecase: now clock is required")
	}
	return &SaveArticleAnnotationUsecase{appendPort: appendPort, now: now}
}

type annotationCreatedPayload struct {
	AnnotationID string `json:"annotation_id"`
	ArticleID    string `json:"article_id"`
	Stance       string `json:"stance"`
	Excerpt      string `json:"excerpt"`
	Note         string `json:"note"`
	CreatedAt    string `json:"created_at"`
}

func (u *SaveArticleAnnotationUsecase) Execute(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	in SaveAnnotationInput,
) (*domain.ArticleAnnotation, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidAnnotationInput)
	}
	articleID, err := uuid.Parse(in.ArticleID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid article_id", domain.ErrInvalidAnnotationInput)
	}
	if !domain.IsValidAnnotationStance(in.Stance) {
		return nil, fmt.Errorf("%w: invalid stance %q", domain.ErrInvalidAnnotationInput, in.Stance)
	}
	excerpt, ok := domain.SanitizeMicroText(in.Excerpt)
	if !ok {
		return nil, fmt.Errorf("%w: excerpt exceeds %d characters", domain.ErrInvalidAnnotationInput, domain.MaxCurationTextRunes)
	}
	note, ok := domain.SanitizeMicroText(in.Note)
	if !ok {
		return nil, fmt.Errorf("%w: note exceeds %d characters", domain.ErrInvalidAnnotationInput, domain.MaxCurationTextRunes)
	}
	if excerpt == "" && note == "" {
		return nil, fmt.Errorf("%w: either excerpt or note is required", domain.ErrInvalidAnnotationInput)
	}
	clientEventID, err := uuid.Parse(in.ClientEventID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid client_event_id", domain.ErrInvalidAnnotationInput)
	}

	receivedAt := u.now().UTC()
	ts := in.CreatedAt
	if ts.IsZero() {
		ts = receivedAt
	}
	clampedAt, _ := domain.ClampReadingOccurredAt(ts, receivedAt)

	// Deterministic annotation_id from client_event_id ensures retried deliveries
	// are strictly idempotent across both dedupe and projection upserts.
	annotationID := clientEventID

	payloadBytes, err := json.Marshal(annotationCreatedPayload{
		AnnotationID: annotationID.String(),
		ArticleID:    articleID.String(),
		Stance:       string(in.Stance),
		Excerpt:      excerpt,
		Note:         note,
		CreatedAt:    clampedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal reading.annotation_created.v1: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingAnnotationCreated,
		AggregateType: domain.AggregateReadingAnnotation,
		AggregateID:   annotationID.String(),
		DedupeKey:     "reading.annotation_created:" + clientEventID.String(),
		Payload:       payloadBytes,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return nil, fmt.Errorf("save article annotation: %w", err)
	}

	return &domain.ArticleAnnotation{
		AnnotationID: annotationID,
		ArticleID:    articleID,
		Stance:       in.Stance,
		Excerpt:      excerpt,
		Note:         note,
		CreatedAt:    clampedAt,
	}, nil
}

type DeleteArticleAnnotationUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewDeleteArticleAnnotationUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *DeleteArticleAnnotationUsecase {
	if appendPort == nil {
		panic("reading_curation_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_curation_usecase: now clock is required")
	}
	return &DeleteArticleAnnotationUsecase{appendPort: appendPort, now: now}
}

type annotationDeletedPayload struct {
	AnnotationID string `json:"annotation_id"`
	DeletedAt    string `json:"deleted_at"`
}

func (u *DeleteArticleAnnotationUsecase) Execute(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	rawAnnotationID, rawClientEventID string,
	deletedAt time.Time,
) error {
	if userID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", domain.ErrInvalidAnnotationInput)
	}
	annotationID, err := uuid.Parse(rawAnnotationID)
	if err != nil {
		return fmt.Errorf("%w: invalid annotation_id", domain.ErrInvalidAnnotationInput)
	}
	clientEventID, err := uuid.Parse(rawClientEventID)
	if err != nil {
		return fmt.Errorf("%w: invalid client_event_id", domain.ErrInvalidAnnotationInput)
	}
	receivedAt := u.now().UTC()
	if deletedAt.IsZero() {
		deletedAt = receivedAt
	}
	clampedAt, _ := domain.ClampReadingOccurredAt(deletedAt, receivedAt)

	payloadBytes, err := json.Marshal(annotationDeletedPayload{
		AnnotationID: annotationID.String(),
		DeletedAt:    clampedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal reading.annotation_deleted.v1: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingAnnotationDeleted,
		AggregateType: domain.AggregateReadingAnnotation,
		AggregateID:   annotationID.String(),
		DedupeKey:     "reading.annotation_deleted:" + clientEventID.String(),
		Payload:       payloadBytes,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return fmt.Errorf("delete article annotation: %w", err)
	}
	return nil
}

type ListArticleAnnotationsUsecase struct {
	port reading_curation_port.ListArticleAnnotationsPort
}

func NewListArticleAnnotationsUsecase(port reading_curation_port.ListArticleAnnotationsPort) *ListArticleAnnotationsUsecase {
	if port == nil {
		panic("reading_curation_usecase: ListArticleAnnotationsPort is required")
	}
	return &ListArticleAnnotationsUsecase{port: port}
}

func (u *ListArticleAnnotationsUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	rawArticleIDs []string,
	limit int32,
) ([]domain.ArticleAnnotation, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidAnnotationInput)
	}
	articleIDs := make([]uuid.UUID, 0, len(rawArticleIDs))
	for _, raw := range rawArticleIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid article_id %q", domain.ErrInvalidAnnotationInput, raw)
		}
		articleIDs = append(articleIDs, id)
	}
	if limit <= 0 {
		limit = defaultListLimit
	} else if limit > maxListLimit {
		limit = maxListLimit
	}
	return u.port.ListArticleAnnotations(ctx, userID, articleIDs, limit)
}

type CurateThemeItemInput struct {
	ThemeID             string
	Kind                domain.ThemeCurationKind
	TargetHash          string
	TargetText          string
	UserNote            string
	AugurConversationID string
	ClientEventID       string
	CreatedAt           time.Time
}

type CurateThemeItemUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewCurateThemeItemUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *CurateThemeItemUsecase {
	if appendPort == nil {
		panic("reading_curation_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_curation_usecase: now clock is required")
	}
	return &CurateThemeItemUsecase{appendPort: appendPort, now: now}
}

type themeItemCuratedPayload struct {
	CurationID          string `json:"curation_id"`
	ThemeID             string `json:"theme_id"`
	ActionKind          string `json:"action_kind"`
	TargetHash          string `json:"target_hash"`
	TargetText          string `json:"target_text"`
	UserNote            string `json:"user_note"`
	AugurConversationID string `json:"augur_conversation_id"`
	CreatedAt           string `json:"created_at"`
}

func (u *CurateThemeItemUsecase) Execute(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	in CurateThemeItemInput,
) (*domain.ThemeCuration, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidCurationInput)
	}
	themeID, err := uuid.Parse(in.ThemeID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid theme_id", domain.ErrInvalidCurationInput)
	}
	if !domain.IsValidThemeCurationKind(in.Kind) {
		return nil, fmt.Errorf("%w: invalid curation kind %q", domain.ErrInvalidCurationInput, in.Kind)
	}
	targetText, ok := domain.SanitizeMicroText(in.TargetText)
	if !ok {
		return nil, fmt.Errorf("%w: target_text exceeds %d characters", domain.ErrInvalidCurationInput, domain.MaxCurationTextRunes)
	}
	userNote, ok := domain.SanitizeMicroText(in.UserNote)
	if !ok {
		return nil, fmt.Errorf("%w: user_note exceeds %d characters", domain.ErrInvalidCurationInput, domain.MaxCurationTextRunes)
	}
	if targetText == "" && userNote == "" {
		return nil, fmt.Errorf("%w: target_text or user_note is required", domain.ErrInvalidCurationInput)
	}

	targetHash := strings.TrimSpace(in.TargetHash)
	if len(targetHash) > 64 {
		return nil, fmt.Errorf("%w: target_hash too long", domain.ErrInvalidCurationInput)
	}
	if targetHash == "" && targetText != "" {
		targetHash = domain.HashTargetSentence(targetText)
	}

	augurConvID := strings.TrimSpace(in.AugurConversationID)
	if augurConvID != "" {
		if _, err := uuid.Parse(augurConvID); err != nil {
			return nil, fmt.Errorf("%w: invalid augur_conversation_id", domain.ErrInvalidCurationInput)
		}
	}

	clientEventID, err := uuid.Parse(in.ClientEventID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid client_event_id", domain.ErrInvalidCurationInput)
	}

	receivedAt := u.now().UTC()
	ts := in.CreatedAt
	if ts.IsZero() {
		ts = receivedAt
	}
	clampedAt, _ := domain.ClampReadingOccurredAt(ts, receivedAt)

	curationID := clientEventID

	payloadBytes, err := json.Marshal(themeItemCuratedPayload{
		CurationID:          curationID.String(),
		ThemeID:             themeID.String(),
		ActionKind:          string(in.Kind),
		TargetHash:          targetHash,
		TargetText:          targetText,
		UserNote:            userNote,
		AugurConversationID: augurConvID,
		CreatedAt:           clampedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal reading.theme_item_curated.v1: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingThemeItemCurated,
		AggregateType: domain.AggregateReadingThemeCuration,
		AggregateID:   curationID.String(),
		DedupeKey:     "reading.theme_item_curated:" + clientEventID.String(),
		Payload:       payloadBytes,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return nil, fmt.Errorf("curate theme item: %w", err)
	}

	return &domain.ThemeCuration{
		CurationID:          curationID,
		ThemeID:             themeID,
		Kind:                in.Kind,
		TargetHash:          targetHash,
		TargetText:          targetText,
		UserNote:            userNote,
		AugurConversationID: augurConvID,
		CreatedAt:           clampedAt,
	}, nil
}

type RemoveThemeCurationUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewRemoveThemeCurationUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *RemoveThemeCurationUsecase {
	if appendPort == nil {
		panic("reading_curation_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_curation_usecase: now clock is required")
	}
	return &RemoveThemeCurationUsecase{appendPort: appendPort, now: now}
}

type themeCurationRemovedPayload struct {
	CurationID string `json:"curation_id"`
	RemovedAt  string `json:"removed_at"`
}

func (u *RemoveThemeCurationUsecase) Execute(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	rawCurationID, rawClientEventID string,
	removedAt time.Time,
) error {
	if userID == uuid.Nil {
		return fmt.Errorf("%w: user_id is required", domain.ErrInvalidCurationInput)
	}
	curationID, err := uuid.Parse(rawCurationID)
	if err != nil {
		return fmt.Errorf("%w: invalid curation_id", domain.ErrInvalidCurationInput)
	}
	clientEventID, err := uuid.Parse(rawClientEventID)
	if err != nil {
		return fmt.Errorf("%w: invalid client_event_id", domain.ErrInvalidCurationInput)
	}
	receivedAt := u.now().UTC()
	if removedAt.IsZero() {
		removedAt = receivedAt
	}
	clampedAt, _ := domain.ClampReadingOccurredAt(removedAt, receivedAt)

	payloadBytes, err := json.Marshal(themeCurationRemovedPayload{
		CurationID: curationID.String(),
		RemovedAt:  clampedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal reading.theme_curation_removed.v1: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingThemeCurationRemove,
		AggregateType: domain.AggregateReadingThemeCuration,
		AggregateID:   curationID.String(),
		DedupeKey:     "reading.theme_curation_removed:" + clientEventID.String(),
		Payload:       payloadBytes,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return fmt.Errorf("remove theme curation: %w", err)
	}
	return nil
}

type ListThemeCurationsUsecase struct {
	port reading_curation_port.ListThemeCurationsPort
}

func NewListThemeCurationsUsecase(port reading_curation_port.ListThemeCurationsPort) *ListThemeCurationsUsecase {
	if port == nil {
		panic("reading_curation_usecase: ListThemeCurationsPort is required")
	}
	return &ListThemeCurationsUsecase{port: port}
}

func (u *ListThemeCurationsUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	rawThemeID string,
	limit int32,
) ([]domain.ThemeCuration, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidCurationInput)
	}
	themeID, err := uuid.Parse(rawThemeID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid theme_id", domain.ErrInvalidCurationInput)
	}
	if limit <= 0 {
		limit = defaultListLimit
	} else if limit > maxListLimit {
		limit = maxListLimit
	}
	return u.port.ListThemeCurations(ctx, userID, themeID, limit)
}
