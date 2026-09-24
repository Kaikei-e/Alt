package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	neturl "net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── fold functions ──
//
// Each fold derives every business fact (timestamps, scores) from the
// event's own OccurredAt/payload — never wall clock, never a read model —
// so replaying the same log reproduces identical rows (reproject-safe).

// ── ArticleCreated ──

// buildArticleCreatedWrites is a pure function that transforms an ArticleCreated
// event into the homeItemWrite and digestWrite models.
func buildArticleCreatedWrites(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, digestWrite, error) {
	var payload articleCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, digestWrite{}, fmt.Errorf("unmarshal ArticleCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, digestWrite{}, fmt.Errorf("parse article_id: %w", err)
	}

	occurredAt := evt.OccurredAt
	var publishedAt *time.Time
	if payload.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, payload.PublishedAt); err == nil {
			publishedAt = &t
		}
	}

	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:       userID,
		TenantID:     evt.TenantID,
		ItemKey:      fmt.Sprintf("article:%s", articleID),
		ItemType:     itemTypeArticle,
		PrimaryRefID: &articleID,
		Title:        payload.Title,
		URL:          payload.URL,
		WhyReasons:   []whyReasonWire{{Code: whyNewUnread}},
		// baseQualityScore is time-invariant by design: how stale
		// published_at already was at ingest is not a fact worth freezing
		// into the stored score forever (the old (occurredAt-publishedAt)
		// decay formula did exactly that, and the merge-safe UPSERT's
		// GREATEST held the winning value permanently — read-time recency
		// ranking belongs in GetKnowledgeHomeItems, over the published_at
		// column stored below, not here).
		Score:             baseQualityScore,
		ScoreOp:           scoreOpMax,
		FreshnessAt:       &occurredAt,
		PublishedAt:       publishedAt,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		SummaryState:      summaryStatePending,
		ProjectionVersion: version,
	}

	digest := digestWrite{
		UserID:               userID,
		DigestDate:           occurredAt.Format(time.DateOnly),
		NewArticles:          1,
		UnsummarizedArticles: 1,
		UpdatedAt:            occurredAt,
		LastEventSeq:         evt.EventSeq,
	}
	return item, digest, nil
}

func (p *Projector) foldArticleCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, digest, err := buildArticleCreatedWrites(evt, version)
	if err != nil {
		return err
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}
	return p.upsertDigest(ctx, digest)
}

// ── ArticleUrlBackfilled ──

// isHTTPURL allowlists {http, https}. Mirrors the FE-side safeArticleHref
// guard so a dangerous scheme rejected on the FE never sneaks back via the
// corrective event.
func isHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Host != ""
	default:
		return false
	}
}

// buildURLPatch is a pure builder constructing the single-column URL patch payload.
func buildURLPatch(evt sovereign_db.KnowledgeEvent, payload articleUrlBackfilledPayload, version int) (sovereign_db.PatchKnowledgeHomeItemURLPayload, error) {
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return sovereign_db.PatchKnowledgeHomeItemURLPayload{}, fmt.Errorf("parse article_id: %w", err)
	}

	patch := sovereign_db.PatchKnowledgeHomeItemURLPayload{
		UserID:            resolveUserID(evt).String(),
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ProjectionVersion: version,
		URL:               payload.URL,
	}
	return patch, nil
}

// foldArticleUrlBackfilled patches only the `url` column of the matching
// knowledge_home_items row — a single-column corrective patch, distinct
// from the full UpsertKnowledgeHomeItem merge-safe upsert.
func (p *Projector) foldArticleUrlBackfilled(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	var payload articleUrlBackfilledPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ArticleUrlBackfilled payload: %w", err)
	}
	if !isHTTPURL(payload.URL) {
		p.logger.WarnContext(ctx, "knowledge_home_projector: skipping ArticleUrlBackfilled with non-HTTP URL",
			slog.String("event_id", evt.EventID.String()), slog.String("article_id", payload.ArticleID))
		return nil
	}
	patch, err := buildURLPatch(evt, payload, version)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal ArticleUrlBackfilled patch: %w", err)
	}
	if err := p.repo.PatchKnowledgeHomeItemURL(ctx, raw); err != nil {
		return fmt.Errorf("patch knowledge_home_items.url: %w", err)
	}
	return nil
}

// ── SummaryVersionCreated ──

// buildSummaryVersionCreatedWrites is a pure function that creates homeItemWrite and digestWrite
// for SummaryVersionCreated events. Design change (F-01) — summary_text travels on
// the event payload itself, so the fold uses it as-is with no alt-db
// GetSummaryVersionByID round trip and no excerpt truncation.
func buildSummaryVersionCreatedWrites(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, digestWrite, error) {
	var payload summaryVersionCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, digestWrite{}, fmt.Errorf("unmarshal SummaryVersionCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, digestWrite{}, fmt.Errorf("parse article_id: %w", err)
	}

	occurredAt := evt.OccurredAt
	summaryState := summaryStatePending
	whyReasons := []whyReasonWire{{Code: whyNewUnread}}
	if payload.SummaryText != "" {
		summaryState = summaryStateReady
		whyReasons = append(whyReasons, whyReasonWire{Code: whySummaryCompleted})
	}

	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:            userID,
		TenantID:          evt.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          itemTypeArticle,
		SummaryExcerpt:    payload.SummaryText,
		SummaryState:      summaryState,
		WhyReasons:        whyReasons,
		Score:             0.8, // boost for having a summary
		ScoreOp:           scoreOpMax,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: version,
	}

	summarizedArticles, unsummarizedDelta := 0, 0
	if summaryState == summaryStateReady {
		summarizedArticles, unsummarizedDelta = 1, -1
	}
	digest := digestWrite{
		UserID:               userID,
		DigestDate:           occurredAt.Format(time.DateOnly),
		SummarizedArticles:   summarizedArticles,
		UnsummarizedArticles: unsummarizedDelta,
		UpdatedAt:            occurredAt,
		LastEventSeq:         evt.EventSeq,
	}
	return item, digest, nil
}

func (p *Projector) foldSummaryVersionCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, digest, err := buildSummaryVersionCreatedWrites(evt, version)
	if err != nil {
		return err
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}
	return p.upsertDigest(ctx, digest)
}

// ── TagSetVersionCreated ──

// buildTagSetVersionCreatedWrites is a pure function that generates homeItemWrite and optional
// digestWrite for TagSetVersionCreated events. Design change (F-01) — tags travel on the event
// payload itself, so the fold uses them as-is with no alt-db GetTagSetVersionByID round trip.
func buildTagSetVersionCreatedWrites(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, *digestWrite, error) {
	var payload tagSetVersionCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, nil, fmt.Errorf("unmarshal TagSetVersionCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, nil, fmt.Errorf("parse article_id: %w", err)
	}

	occurredAt := evt.OccurredAt
	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:            userID,
		TenantID:          evt.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          itemTypeArticle,
		Tags:              payload.Tags,
		WhyReasons:        []whyReasonWire{{Code: whyNewUnread}},
		Score:             0.7,
		ScoreOp:           scoreOpMax,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: version,
	}

	// today_digest_view.top_tags is merge-safe (COALESCE on empty), but an
	// empty tag set must skip the upsert entirely rather than send a no-op —
	// touching the row would still bump its updated_at.
	var digest *digestWrite
	if len(payload.Tags) > 0 {
		digest = &digestWrite{
			UserID:       userID,
			DigestDate:   occurredAt.Format(time.DateOnly),
			TopTags:      payload.Tags,
			UpdatedAt:    occurredAt,
			LastEventSeq: evt.EventSeq,
		}
	}
	return item, digest, nil
}

func (p *Projector) foldTagSetVersionCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, digest, err := buildTagSetVersionCreatedWrites(evt, version)
	if err != nil {
		return err
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}
	if digest != nil {
		return p.upsertDigest(ctx, *digest)
	}
	return nil
}
