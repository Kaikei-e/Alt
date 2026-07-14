// Package knowledge_home_projector folds the Knowledge Sovereign append-only
// event log into the Knowledge Home read models (knowledge_home_items,
// today_digest_view, recall_candidate_view). It is reproject-safe: every
// business fact is derived from a single event's OccurredAt and
// payload-resident fields, never from alt-db lookups or other read models.
// Re-running over the same log reproduces the same read models.
//
// Ported from alt-backend/app/job/knowledge_projector.go per F-01
// (docs/review/architecturereview20260713.md): the fold logic used to live
// in alt-backend and reach knowledge-sovereign over an ApplyProjectionMutation
// RPC round trip. Moving it in-process (mirroring knowledge_trail_projector)
// removes that RPC hop. SummaryVersionCreated and TagSetVersionCreated no
// longer read the summary/tag body back from alt-db (GetSummaryVersionByID /
// GetTagSetVersionByID) — the producer now carries summary_text / tags on
// the event payload itself, and the fold uses them as-is.
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

const (
	projectorName    = "knowledge-home-projector"
	defaultBatchSize = 100
	defaultMaxTick   = 4

	itemTypeArticle = "article"

	whyNewUnread        = "new_unread"
	whySummaryCompleted = "summary_completed"

	summaryStatePending = "pending"
	summaryStateReady   = "ready"

	supersedeSummaryUpdated = "summary_updated"
	supersedeTagsUpdated    = "tags_updated"
	supersedeReasonUpdated  = "reason_updated"

	recallReasonOpenedNotRevisited = "opened_before_but_not_revisited"

	// currentProjectionVersion is hardcoded because this narrow Repository
	// interface has no GetActiveVersion port (unlike alt-backend's
	// KnowledgeProjectorJob, which resolved an active shadow/live version).
	// That concern does not yet exist in this package's scope.
	currentProjectionVersion = 1
)

// Repository is the narrow surface the projector needs. Every write method
// takes the same json.RawMessage envelope sovereign_db.Repository already
// exposes for ApplyProjectionMutation, so *sovereign_db.Repository satisfies
// this interface directly — no driver changes, no intermediate gateway,
// same direct-wiring shape as knowledge_trail_projector.Repository.
type Repository interface {
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	DismissKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	ClearSupersedeState(ctx context.Context, payload json.RawMessage) error
	UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error
	UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error
	PatchKnowledgeHomeItemURL(ctx context.Context, payload json.RawMessage) error
}

var _ Repository = (*sovereign_db.Repository)(nil)

// Config tunes batch sizing.
type Config struct {
	BatchSize         int
	MaxBatchesPerTick int
}

// Projector folds events into the Knowledge Home read models.
type Projector struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
}

// NewProjector builds a Projector. logger defaults to slog.Default() when
// nil; BatchSize/MaxBatchesPerTick default when <= 0.
func NewProjector(repo Repository, logger *slog.Logger, cfg Config) *Projector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.MaxBatchesPerTick <= 0 {
		cfg.MaxBatchesPerTick = defaultMaxTick
	}
	return &Projector{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch drains up to MaxBatchesPerTick batches from the event log, folding
// each of the 9 Knowledge Home event types (ArticleCreated,
// ArticleUrlBackfilled, SummaryVersionCreated, TagSetVersionCreated,
// HomeItemOpened, HomeItemDismissed, SummarySuperseded, TagSetSuperseded,
// ReasonMerged) into the read models and advancing the checkpoint.
//
// A malformed/unparseable payload stops the batch without advancing the
// checkpoint past the failing event (it stays in the log to be retried),
// mirroring alt-backend/app/job/knowledge_projector.go's hard-fail-and-stop
// semantics. Unknown event types are skipped but still advance the
// checkpoint.
func (p *Projector) RunBatch(ctx context.Context) error {
	for i := 0; i < p.cfg.MaxBatchesPerTick; i++ {
		checkpoint, err := p.repo.GetProjectionCheckpoint(ctx, projectorName)
		if err != nil {
			return fmt.Errorf("get checkpoint: %w", err)
		}
		events, err := p.repo.ListKnowledgeEventsSince(ctx, checkpoint, p.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}
		if len(events) == 0 {
			return nil
		}

		lastGoodSeq := checkpoint
		var foldErr error
		for _, evt := range events {
			if err := p.foldEvent(ctx, evt); err != nil {
				foldErr = fmt.Errorf("fold event %s (seq=%d): %w", evt.EventType, evt.EventSeq, err)
				break
			}
			lastGoodSeq = evt.EventSeq
		}

		// Advance the checkpoint up to (and including) the last
		// successfully-folded event even when the batch stopped early on a
		// hard failure — the failing event itself is never skipped past.
		if lastGoodSeq > checkpoint {
			if err := p.repo.UpdateProjectionCheckpoint(ctx, projectorName, lastGoodSeq); err != nil {
				return fmt.Errorf("update checkpoint: %w", err)
			}
		}
		if foldErr != nil {
			return foldErr
		}
		if len(events) < p.cfg.BatchSize {
			return nil
		}
	}
	return nil
}

func (p *Projector) foldEvent(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	switch evt.EventType {
	case "ArticleCreated":
		return p.foldArticleCreated(ctx, evt)
	case "ArticleUrlBackfilled":
		return p.foldArticleUrlBackfilled(ctx, evt)
	case "SummaryVersionCreated":
		return p.foldSummaryVersionCreated(ctx, evt)
	case "TagSetVersionCreated":
		return p.foldTagSetVersionCreated(ctx, evt)
	case "HomeItemOpened":
		return p.foldHomeItemOpened(ctx, evt)
	case "HomeItemDismissed":
		return p.foldHomeItemDismissed(ctx, evt)
	case "SummarySuperseded":
		return p.foldSummarySuperseded(ctx, evt)
	case "TagSetSuperseded":
		return p.foldTagSetSuperseded(ctx, evt)
	case "ReasonMerged":
		return p.foldReasonMerged(ctx, evt)
	default:
		// Unknown event types are silently skipped but still advance the
		// checkpoint (handled by the caller).
		return nil
	}
}

// resolveUserID falls back to TenantID for tenant-wide system events that
// carry no user_id (mirrors alt-backend/app/job/knowledge_projector.go).
func resolveUserID(evt sovereign_db.KnowledgeEvent) uuid.UUID {
	if evt.UserID != nil {
		return *evt.UserID
	}
	return evt.TenantID
}

// ── wire-write types ──
//
// These mirror the exact json.RawMessage unmarshal targets sovereign_db.
// Repository's mutation methods already expose (see repository.go and
// patch_knowledge_home_item_url.go). Field names/tags must match those
// targets verbatim — Repository is satisfied directly by
// *sovereign_db.Repository, with no intermediate gateway to translate shape.

type whyReasonWire struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type homeItemWrite struct {
	UserID            uuid.UUID       `json:"user_id"`
	TenantID          uuid.UUID       `json:"tenant_id"`
	ItemKey           string          `json:"item_key"`
	ItemType          string          `json:"item_type"`
	PrimaryRefID      *uuid.UUID      `json:"primary_ref_id"`
	Title             string          `json:"title"`
	SummaryExcerpt    string          `json:"summary_excerpt"`
	Tags              []string        `json:"tags"`
	WhyReasons        []whyReasonWire `json:"why_reasons"`
	Score             float64         `json:"score"`
	FreshnessAt       *time.Time      `json:"freshness_at"`
	PublishedAt       *time.Time      `json:"published_at"`
	LastInteractedAt  *time.Time      `json:"last_interacted_at"`
	GeneratedAt       time.Time       `json:"generated_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	DismissedAt       *time.Time      `json:"dismissed_at"`
	ProjectionVersion int             `json:"projection_version"`
	SummaryState      string          `json:"summary_state"`
	SupersedeState    string          `json:"supersede_state"`
	SupersededAt      *time.Time      `json:"superseded_at"`
	PreviousRefJSON   string          `json:"previous_ref_json"`
	URL               string          `json:"url"`
}

type dismissWrite struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	DismissedAt       string `json:"dismissed_at"`
}

type clearSupersedeWrite struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
}

type digestWrite struct {
	UserID               uuid.UUID `json:"user_id"`
	DigestDate           string    `json:"digest_date"`
	NewArticles          int       `json:"new_articles"`
	SummarizedArticles   int       `json:"summarized_articles"`
	UnsummarizedArticles int       `json:"unsummarized_articles"`
	TopTags              []string  `json:"top_tags"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type recallReasonWire struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type recallCandidateWrite struct {
	UserID            uuid.UUID          `json:"user_id"`
	ItemKey           string             `json:"item_key"`
	Reasons           []recallReasonWire `json:"reasons"`
	RecallScore       float64            `json:"recall_score"`
	NextSuggestAt     *time.Time         `json:"next_suggest_at"`
	FirstEligibleAt   *time.Time         `json:"first_eligible_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	ProjectionVersion int                `json:"projection_version"`
}

// ── incoming event payload shapes ──

type articleCreatedPayload struct {
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
}

type articleUrlBackfilledPayload struct {
	ArticleID string `json:"article_id"`
	URL       string `json:"url"`
}

type summaryVersionCreatedPayload struct {
	ArticleID   string `json:"article_id"`
	SummaryText string `json:"summary_text"`
}

type tagSetVersionCreatedPayload struct {
	ArticleID string   `json:"article_id"`
	Tags      []string `json:"tags"`
}

type homeItemOpenedPayload struct {
	ItemKey string `json:"item_key"`
}

type homeItemDismissedPayload struct {
	ItemKey string `json:"item_key"`
}

type summarySupersededPayload struct {
	ArticleID              string `json:"article_id"`
	PreviousSummaryExcerpt string `json:"previous_summary_excerpt"`
}

type tagSetSupersededPayload struct {
	ArticleID    string   `json:"article_id"`
	PreviousTags []string `json:"previous_tags"`
}

type reasonMergedPayload struct {
	ArticleID        string   `json:"article_id"`
	ItemKey          string   `json:"item_key"`
	PreviousWhyCodes []string `json:"previous_why_codes"`
}

// ── repository call helpers ──

func (p *Projector) upsertHomeItem(ctx context.Context, item homeItemWrite) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal knowledge_home_items payload: %w", err)
	}
	if err := p.repo.UpsertKnowledgeHomeItem(ctx, raw); err != nil {
		return fmt.Errorf("upsert knowledge_home_items: %w", err)
	}
	return nil
}

func (p *Projector) upsertDigest(ctx context.Context, digest digestWrite) error {
	raw, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("marshal today_digest_view payload: %w", err)
	}
	return p.repo.UpsertTodayDigest(ctx, raw)
}

func (p *Projector) upsertRecallCandidate(ctx context.Context, candidate recallCandidateWrite) error {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("marshal recall_candidate_view payload: %w", err)
	}
	return p.repo.UpsertRecallCandidate(ctx, raw)
}

func (p *Projector) clearSupersedeState(ctx context.Context, params clearSupersedeWrite) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal clear_supersede_state payload: %w", err)
	}
	return p.repo.ClearSupersedeState(ctx, raw)
}

// ── fold functions ──
//
// Each fold derives every business fact (timestamps, scores) from the
// event's own OccurredAt/payload — never wall-clock, never a read model —
// so replaying the same log reproduces identical rows (reproject-safe).

func (p *Projector) foldArticleCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload articleCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ArticleCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	occurredAt := evt.OccurredAt
	var publishedAt *time.Time
	if payload.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, payload.PublishedAt); err == nil {
			publishedAt = &t
		}
	}

	// Freshness decay: newer publishedAt = higher score. Anchored on
	// event.OccurredAt (not wall-clock) so replay is bit-identical.
	score := 1.0
	if publishedAt != nil {
		hoursOld := occurredAt.Sub(*publishedAt).Hours()
		switch {
		case hoursOld < 24:
			score = 1.0 - (hoursOld / 48.0)
		case hoursOld > 0:
			score = 0.5 / (hoursOld / 24.0)
		}
	}

	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:            userID,
		TenantID:          evt.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          itemTypeArticle,
		PrimaryRefID:      &articleID,
		Title:             payload.Title,
		URL:               payload.URL,
		WhyReasons:        []whyReasonWire{{Code: whyNewUnread}},
		Score:             score,
		FreshnessAt:       &occurredAt,
		PublishedAt:       publishedAt,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		SummaryState:      summaryStatePending,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}

	digest := digestWrite{
		UserID:               userID,
		DigestDate:           occurredAt.Format(time.DateOnly),
		NewArticles:          1,
		UnsummarizedArticles: 1,
		UpdatedAt:            occurredAt,
	}
	if err := p.upsertDigest(ctx, digest); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: today_digest upsert failed for ArticleCreated",
			slog.String("event_id", evt.EventID.String()), slog.Any("error", err))
	}
	return nil
}

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

// foldArticleUrlBackfilled patches only the `url` column of the matching
// knowledge_home_items row — a single-column corrective patch, distinct
// from the full UpsertKnowledgeHomeItem merge-safe upsert.
func (p *Projector) foldArticleUrlBackfilled(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload articleUrlBackfilledPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ArticleUrlBackfilled payload: %w", err)
	}
	if !isHTTPURL(payload.URL) {
		p.logger.WarnContext(ctx, "knowledge_home_projector: skipping ArticleUrlBackfilled with non-HTTP URL",
			slog.String("event_id", evt.EventID.String()), slog.String("article_id", payload.ArticleID))
		return nil
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}

	patch := sovereign_db.PatchKnowledgeHomeItemURLPayload{
		UserID:            resolveUserID(evt).String(),
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ProjectionVersion: currentProjectionVersion,
		URL:               payload.URL,
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

// foldSummaryVersionCreated: design change (F-01) — summary_text travels on
// the event payload itself, so the fold uses it as-is with no alt-db
// GetSummaryVersionByID round trip and no excerpt truncation.
func (p *Projector) foldSummaryVersionCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload summaryVersionCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SummaryVersionCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
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
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
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
	}
	if err := p.upsertDigest(ctx, digest); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: today_digest upsert failed for SummaryVersionCreated",
			slog.String("event_id", evt.EventID.String()), slog.Any("error", err))
	}
	return nil
}

// foldTagSetVersionCreated: design change (F-01) — tags travel on the event
// payload itself, so the fold uses them as-is with no alt-db
// GetTagSetVersionByID round trip and no parseTagNames.
func (p *Projector) foldTagSetVersionCreated(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload tagSetVersionCreatedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal TagSetVersionCreated payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
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
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}

	// today_digest_view.top_tags is merge-safe (COALESCE on empty), but an
	// empty tag set must skip the upsert entirely rather than send a no-op —
	// touching the row would still bump its updated_at.
	if len(payload.Tags) > 0 {
		digest := digestWrite{
			UserID:     userID,
			DigestDate: occurredAt.Format(time.DateOnly),
			TopTags:    payload.Tags,
			UpdatedAt:  occurredAt,
		}
		if err := p.upsertDigest(ctx, digest); err != nil {
			p.logger.WarnContext(ctx, "knowledge_home_projector: today_digest upsert failed for TagSetVersionCreated",
				slog.String("event_id", evt.EventID.String()), slog.Any("error", err))
		}
	}
	return nil
}

func (p *Projector) foldHomeItemOpened(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload homeItemOpenedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal HomeItemOpened payload: %w", err)
	}

	occurredAt := evt.OccurredAt
	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:            userID,
		TenantID:          evt.TenantID,
		ItemKey:           payload.ItemKey,
		ItemType:          itemTypeArticle,
		WhyReasons:        []whyReasonWire{{Code: whyNewUnread}},
		Score:             0.1, // suppressed: opening an item lowers its resurfacing score
		LastInteractedAt:  &occurredAt,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}

	// Clear supersede state on open (acknowledgement). Non-fatal.
	clear := clearSupersedeWrite{
		UserID:            userID.String(),
		ItemKey:           payload.ItemKey,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.clearSupersedeState(ctx, clear); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: clear supersede state failed on open",
			slog.String("event_id", evt.EventID.String()), slog.String("item_key", payload.ItemKey), slog.Any("error", err))
	}

	// Recall candidate: eligible 1h after the event's own time. Non-fatal.
	eligibleAt := occurredAt.Add(1 * time.Hour)
	candidate := recallCandidateWrite{
		UserID:            userID,
		ItemKey:           payload.ItemKey,
		RecallScore:       0.5,
		Reasons:           []recallReasonWire{{Type: recallReasonOpenedNotRevisited, Description: "Opened but not revisited"}},
		FirstEligibleAt:   &eligibleAt,
		NextSuggestAt:     &eligibleAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	if err := p.upsertRecallCandidate(ctx, candidate); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: recall candidate upsert failed",
			slog.String("event_id", evt.EventID.String()), slog.Any("error", err))
	}
	return nil
}

func (p *Projector) foldHomeItemDismissed(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload homeItemDismissedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal HomeItemDismissed payload: %w", err)
	}
	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = evt.AggregateID
	}
	if itemKey == "" {
		return fmt.Errorf("home item dismiss payload missing item_key")
	}

	dismiss := dismissWrite{
		UserID:            resolveUserID(evt).String(),
		ItemKey:           itemKey,
		ProjectionVersion: currentProjectionVersion,
		// Business fact from the event only — knowledge_events.occurred_at is
		// NOT NULL, so there is no wall-clock fallback here (reproject-safe).
		DismissedAt: evt.OccurredAt.Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(dismiss)
	if err != nil {
		return fmt.Errorf("marshal HomeItemDismissed payload: %w", err)
	}
	if err := p.repo.DismissKnowledgeHomeItem(ctx, raw); err != nil {
		return fmt.Errorf("dismiss knowledge_home_items: %w", err)
	}
	return nil
}

// ── supersede folds ──

func (p *Projector) foldSummarySuperseded(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload summarySupersededPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal SummarySuperseded payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}
	prevRef, err := json.Marshal(map[string]string{"previous_summary_excerpt": payload.PreviousSummaryExcerpt})
	if err != nil {
		return fmt.Errorf("marshal previous_summary_excerpt ref: %w", err)
	}

	occurredAt := evt.OccurredAt
	item := homeItemWrite{
		UserID:   resolveUserID(evt),
		TenantID: evt.TenantID,
		ItemKey:  fmt.Sprintf("article:%s", articleID),
		ItemType: itemTypeArticle,
		// Explicit empty slice, not nil — nil serializes to JSON null, which
		// would wipe the row's existing tags via the merge-safe upsert.
		Tags:              []string{},
		SupersedeState:    supersedeSummaryUpdated,
		SupersededAt:      &occurredAt,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	return p.upsertHomeItem(ctx, item)
}

func (p *Projector) foldTagSetSuperseded(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload tagSetSupersededPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal TagSetSuperseded payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}
	prevRef, err := json.Marshal(map[string][]string{"previous_tags": payload.PreviousTags})
	if err != nil {
		return fmt.Errorf("marshal previous_tags ref: %w", err)
	}

	occurredAt := evt.OccurredAt
	item := homeItemWrite{
		UserID:            resolveUserID(evt),
		TenantID:          evt.TenantID,
		ItemKey:           fmt.Sprintf("article:%s", articleID),
		ItemType:          itemTypeArticle,
		Tags:              []string{}, // explicit empty slice — see foldSummarySuperseded
		SupersedeState:    supersedeTagsUpdated,
		SupersededAt:      &occurredAt,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	return p.upsertHomeItem(ctx, item)
}

func (p *Projector) foldReasonMerged(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	var payload reasonMergedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ReasonMerged payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return fmt.Errorf("parse article_id: %w", err)
	}
	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = fmt.Sprintf("article:%s", articleID)
	}
	prevRef, err := json.Marshal(map[string][]string{"previous_why_codes": payload.PreviousWhyCodes})
	if err != nil {
		return fmt.Errorf("marshal previous_why_codes ref: %w", err)
	}

	occurredAt := evt.OccurredAt
	item := homeItemWrite{
		UserID:            resolveUserID(evt),
		TenantID:          evt.TenantID,
		ItemKey:           itemKey,
		ItemType:          itemTypeArticle,
		SupersedeState:    supersedeReasonUpdated,
		SupersededAt:      &occurredAt,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: currentProjectionVersion,
	}
	return p.upsertHomeItem(ctx, item)
}
