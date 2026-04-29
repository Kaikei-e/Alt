// Package knowledge_url_backfill_usecase emits ArticleUrlBackfilled
// corrective events to repair Knowledge Home article rows whose
// projected URL is empty because the original ArticleCreated event
// was written with the legacy "link" wire key (or with no URL key
// at all) — see ADR-000867 / ADR-000868 / PM-2026-041.
//
// Distinct from knowledge_backfill_usecase: that path emits
// ArticleCreated and is silently no-op'd by the dedupe registry
// once articles are already known. This path uses the dedicated
// `article-url-backfill:<article_id>` dedupe namespace and the
// dedicated event type so each emit lands as a fresh event the
// projector can apply via PatchKnowledgeHomeItemURL.
package knowledge_url_backfill_usecase

import (
	"alt/domain"
	"alt/port/knowledge_backfill_port"
	"alt/port/knowledge_event_port"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EmitResult carries the per-invocation summary returned to the caller.
type EmitResult struct {
	ArticlesScanned      int
	EventsAppended       int
	SkippedBlockedScheme int
	SkippedDuplicate     int
	MoreRemaining        bool
}

// Usecase orchestrates the URL backfill emit.
type Usecase struct {
	articlesPort knowledge_backfill_port.ListBackfillArticlesPort
	eventPort    knowledge_event_port.AppendKnowledgeEventPort
}

// NewUsecase wires the URL backfill emitter.
func NewUsecase(
	articles knowledge_backfill_port.ListBackfillArticlesPort,
	events knowledge_event_port.AppendKnowledgeEventPort,
) *Usecase {
	return &Usecase{articlesPort: articles, eventPort: events}
}

// pageSize controls how many rows the underlying SELECT pulls per
// iteration. Each iteration emits one event per row, so this also
// caps the per-iteration sovereign RPC fan-out. 200 is the same
// value knowledge_backfill_job uses — proven not to overload the
// sovereign-side dedupe insert path.
const pageSize = 200

// Emit walks `articles` (cursor-paginated by created_at, id) and
// appends ArticleUrlBackfilled events for every article whose URL is
// non-empty and passes the http(s) scheme allowlist. maxArticles == 0
// means "process every qualifying article". dryRun reports counts but
// does not append any events.
//
// Idempotent: re-running counts SkippedDuplicate for events the
// dedupe registry already had. The sovereign AppendKnowledgeEvent
// returns (0, nil) on dedupe hit; we treat eventSeq==0 as duplicate.
func (u *Usecase) Emit(ctx context.Context, maxArticles int, dryRun bool) (*EmitResult, error) {
	res := &EmitResult{}
	var (
		cursorTime *time.Time
		cursorID   *uuid.UUID
	)
	for {
		articles, err := u.articlesPort.ListBackfillArticles(ctx, cursorTime, cursorID, pageSize)
		if err != nil {
			return res, fmt.Errorf("list backfill articles: %w", err)
		}
		if len(articles) == 0 {
			return res, nil
		}

		for _, a := range articles {
			res.ArticlesScanned++
			if !isHTTPURL(a.URL) {
				res.SkippedBlockedScheme++
			} else if !dryRun {
				appended, err := u.appendCorrective(ctx, a.ArticleID, a.URL, a.CreatedAt, a.UserID)
				if err != nil {
					return res, fmt.Errorf("append corrective for %s: %w", a.ArticleID, err)
				}
				if appended {
					res.EventsAppended++
				} else {
					res.SkippedDuplicate++
				}
			}

			if maxArticles > 0 && res.ArticlesScanned >= maxArticles {
				// Cursor-friendly stop: the next invocation can resume
				// with cursorTime/cursorID once the operator re-runs.
				res.MoreRemaining = true
				return res, nil
			}

			cursorTime = &a.CreatedAt
			id := a.ArticleID
			cursorID = &id
		}

		if len(articles) < pageSize {
			return res, nil
		}
	}
}

// appendCorrective marshals one ArticleUrlBackfilled event with the
// canonical wire form and appends it. Returns (appended, err) where
// appended == false signals dedupe-registry hit (idempotent re-run).
//
// The payload carries `original_occurred_at` = the article's source-row
// `created_at` (RFC3339) per Verraes' multi-temporal events pattern:
// the event's wall-clock OccurredAt records when the corrective event
// was emitted, while the payload's `original_occurred_at` records the
// fact-time when the article was first observed.
func (u *Usecase) appendCorrective(ctx context.Context, articleID uuid.UUID, url string, originalCreatedAt time.Time, userID uuid.UUID) (bool, error) {
	originalOccurredAt := ""
	if !originalCreatedAt.IsZero() {
		originalOccurredAt = originalCreatedAt.UTC().Format(time.RFC3339)
	}
	payload, err := json.Marshal(domain.ArticleUrlBackfilledPayload{
		ArticleID:          articleID.String(),
		URL:                url,
		OriginalOccurredAt: originalOccurredAt,
	})
	if err != nil {
		return false, fmt.Errorf("marshal payload: %w", err)
	}
	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      userID,
		UserID:        &userID,
		ActorType:     domain.ActorService,
		ActorID:       "knowledge-url-backfill",
		EventType:     domain.EventArticleUrlBackfilled,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID.String(),
		DedupeKey:     fmt.Sprintf(domain.DedupeKeyArticleUrlBackfill, articleID.String()),
		Payload:       payload,
	}
	eventSeq, err := u.eventPort.AppendKnowledgeEvent(ctx, event)
	if err != nil {
		return false, err
	}
	// Per the AppendKnowledgeEventPort contract: eventSeq==0 signals
	// the sovereign dedupe registry already had this dedupe_key, so
	// no new event row was written. Treat as idempotent skip — the
	// projector still has the prior corrective patch applied.
	return eventSeq != 0, nil
}

// isHTTPURL allowlist mirrors alt-backend/app/job/knowledge_projector.go
// (projector-side defense) and alt-frontend-sv/src/lib/utils/safeHref.ts
// (FE defense). Three layers, all pinned to {http, https} per
// security-auditor F-001 in ADR-000867.
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
