package reading_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

const (
	eventSessionEnded               = "reading.session_ended.v1"
	eventReadingQueued              = "reading.queued.v1"
	eventReadingUnqueue             = "reading.unqueued.v1"
	eventReadingThemeVersionCreated = "reading.theme_version_created.v1"
	eventReadingThemeOpened         = "reading.theme_opened.v1"
	eventHomeItemOpened             = "HomeItemOpened"

	articlePrefix = "article:"
)

type sessionEndedPayload struct {
	ArticleID       string `json:"article_id"`
	ActiveDwellMs   int64  `json:"active_dwell_ms"`
	BodyLengthChars int32  `json:"body_length_chars"`
	ReachedEnd      bool   `json:"reached_end"`
}

type readingQueuedPayload struct {
	ArticleID          string `json:"article_id"`
	QueuedAt           string `json:"queued_at"`
	ExpiresAt          string `json:"expires_at"`
	QueuePolicyVersion int    `json:"queue_policy_version"`
}

type readingUnqueuedPayload struct {
	ArticleID string `json:"article_id"`
}

type homeItemOpenedPayload struct {
	ItemKey string `json:"item_key"`
}

type readingThemeVersionCreatedPayload struct {
	ThemeID          string   `json:"theme_id"`
	Version          int      `json:"version"`
	Title            string   `json:"title"`
	MemberArticleIDs []string `json:"member_article_ids"`
	ArticleCount     int      `json:"article_count"`
	CreatedAt        string   `json:"created_at"`
}

type readingThemeOpenedPayload struct {
	ThemeID  string `json:"theme_id"`
	Version  int    `json:"version"`
	OpenedAt string `json:"opened_at"`
}

func (p *Projector) foldEvent(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	switch evt.EventType {
	case eventSessionEnded:
		return p.foldSessionEnded(ctx, evt)
	case eventHomeItemOpened:
		return p.foldHomeItemOpened(ctx, evt)
	case eventReadingQueued:
		return p.foldReadingQueued(ctx, evt)
	case eventReadingUnqueue:
		return p.foldReadingUnqueued(ctx, evt)
	case eventReadingThemeVersionCreated:
		return p.foldThemeVersionCreated(ctx, evt)
	case eventReadingThemeOpened:
		return p.foldThemeOpened(ctx, evt)
	default:
		return nil
	}
}

func (p *Projector) foldSessionEnded(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload sessionEndedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", eventSessionEnded, err)
	}
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping session_ended without user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping session_ended with invalid article_id",
			slog.String("event_id", evt.EventID.String()),
			slog.String("article_id", payload.ArticleID))
		return nil //nolint:nilerr // invalid article_id in append-only event must be skipped so projector advances
	}
	if !EvaluateReadRuleV1(payload.ReachedEnd, payload.ActiveDwellMs, payload.BodyLengthChars) {
		return nil
	}
	if err := p.repo.UpsertReadingSource(ctx, sovereign_db.ReadingSource{
		UserID:          *evt.UserID,
		TenantID:        evt.TenantID,
		ArticleID:       articleID,
		FirstReadAt:     evt.OccurredAt,
		ReadRuleVersion: ReadRuleVersionV1,
	}); err != nil {
		return err
	}
	return p.repo.RemoveReadingQueueItem(ctx, *evt.UserID, articleID, evt.OccurredAt, "read", evt.EventSeq)
}

func (p *Projector) foldHomeItemOpened(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	if evt.UserID == nil {
		return nil
	}
	var payload homeItemOpenedPayload
	if len(evt.Payload) > 0 {
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal %s: %w", eventHomeItemOpened, err)
		}
	}
	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = evt.AggregateID
	}
	rawArticleID, ok := strings.CutPrefix(itemKey, articlePrefix)
	if !ok {
		return nil
	}
	articleID, err := uuid.Parse(rawArticleID)
	if err != nil {
		return nil //nolint:nilerr // legacy HomeItemOpened with non-UUID item_key is ignored
	}
	return p.repo.UpsertReadingSource(ctx, sovereign_db.ReadingSource{
		UserID:          *evt.UserID,
		TenantID:        evt.TenantID,
		ArticleID:       articleID,
		FirstReadAt:     evt.OccurredAt,
		ReadRuleVersion: LegacyReadRuleVersion,
	})
}

func (p *Projector) foldReadingQueued(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload readingQueuedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", eventReadingQueued, err)
	}
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping reading.queued without user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("invalid article_id in %s: %w", eventReadingQueued, err)
	}
	queuedAt := evt.OccurredAt
	if payload.QueuedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, payload.QueuedAt)
		if err != nil {
			return fmt.Errorf("invalid queued_at in %s: %w", eventReadingQueued, err)
		}
		queuedAt = parsed
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid expires_at in %s: %w", eventReadingQueued, err)
	}
	if payload.QueuePolicyVersion <= 0 {
		return fmt.Errorf("invalid queue_policy_version %d in %s", payload.QueuePolicyVersion, eventReadingQueued)
	}
	return p.repo.UpsertReadingQueueItem(ctx, sovereign_db.ReadingQueueItem{
		UserID:             *evt.UserID,
		TenantID:           evt.TenantID,
		ArticleID:          articleID,
		QueuedAt:           queuedAt,
		ExpiresAt:          expiresAt,
		QueuePolicyVersion: payload.QueuePolicyVersion,
		LastEventSeq:       evt.EventSeq,
	})
}

func (p *Projector) foldReadingUnqueued(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload readingUnqueuedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", eventReadingUnqueue, err)
	}
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping reading.unqueued without user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("invalid article_id in %s: %w", eventReadingUnqueue, err)
	}
	return p.repo.RemoveReadingQueueItem(ctx, *evt.UserID, articleID, evt.OccurredAt, "unqueued", evt.EventSeq)
}

func (p *Projector) foldThemeVersionCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload readingThemeVersionCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", eventReadingThemeVersionCreated, err)
	}
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping reading.theme_version_created without user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	themeID, err := uuid.Parse(payload.ThemeID)
	if err != nil {
		return fmt.Errorf("invalid theme_id in %s: %w", eventReadingThemeVersionCreated, err)
	}
	if payload.Version < 1 {
		return fmt.Errorf("invalid version %d in %s", payload.Version, eventReadingThemeVersionCreated)
	}
	if payload.ArticleCount < 0 {
		return fmt.Errorf("invalid article_count %d in %s", payload.ArticleCount, eventReadingThemeVersionCreated)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil {
		return fmt.Errorf("invalid created_at in %s: %w", eventReadingThemeVersionCreated, err)
	}

	seen := make(map[uuid.UUID]struct{}, len(payload.MemberArticleIDs))
	memberIDs := make([]uuid.UUID, 0, len(payload.MemberArticleIDs))
	for _, rawID := range payload.MemberArticleIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return fmt.Errorf("invalid member_article_id in %s: %w", eventReadingThemeVersionCreated, err)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		memberIDs = append(memberIDs, id)
	}

	return p.repo.UpsertReadingThemeVersion(ctx, sovereign_db.ReadingTheme{
		UserID:        *evt.UserID,
		TenantID:      evt.TenantID,
		ThemeID:       themeID,
		Title:         payload.Title,
		LatestVersion: payload.Version,
		ArticleCount:  payload.ArticleCount,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}, memberIDs)
}

func (p *Projector) foldThemeOpened(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload readingThemeOpenedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal %s: %w", eventReadingThemeOpened, err)
	}
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "reading_projector: skipping reading.theme_opened without user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	themeID, err := uuid.Parse(payload.ThemeID)
	if err != nil {
		return fmt.Errorf("invalid theme_id in %s: %w", eventReadingThemeOpened, err)
	}
	if payload.Version < 1 {
		return fmt.Errorf("invalid version %d in %s", payload.Version, eventReadingThemeOpened)
	}
	openedAt, err := time.Parse(time.RFC3339Nano, payload.OpenedAt)
	if err != nil {
		return fmt.Errorf("invalid opened_at in %s: %w", eventReadingThemeOpened, err)
	}
	return p.repo.MarkReadingThemeOpened(ctx, *evt.UserID, themeID, openedAt, payload.Version)
}
