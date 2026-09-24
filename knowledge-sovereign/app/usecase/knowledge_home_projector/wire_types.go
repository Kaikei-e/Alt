package knowledge_home_projector

import (
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

	// baseQualityScore is the flat, time-invariant floor a freshly-created
	// article starts at. It intentionally carries no freshness/decay
	// component — see the score_op doc comment on homeItemWrite for why.
	baseQualityScore = 0.5

	// scoreOp values tell sovereign_db.Repository's merge-safe UPSERT how to
	// combine an incoming score with whatever is already stored:
	//   - scoreOpMax: floor semantics — raise the stored score to this
	//     value if it is currently lower, never lower it. Used for the
	//     baseline/boost signals (ArticleCreated, SummaryVersionCreated,
	//     TagSetVersionCreated), which only ever add information.
	//   - scoreOpSet: authoritative overwrite — replace the stored score
	//     unconditionally, including downward. Used for HomeItemOpened's
	//     suppression, which must be able to lower a score.
	//   - "" (zero value, left unset by folds that never touch score, e.g.
	//     the supersede folds): leave the stored score untouched.
	scoreOpMax = "max"
	scoreOpSet = "set"
)

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
	UserID         uuid.UUID       `json:"user_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	ItemKey        string          `json:"item_key"`
	ItemType       string          `json:"item_type"`
	PrimaryRefID   *uuid.UUID      `json:"primary_ref_id"`
	Title          string          `json:"title"`
	SummaryExcerpt string          `json:"summary_excerpt"`
	Tags           []string        `json:"tags"`
	WhyReasons     []whyReasonWire `json:"why_reasons"`
	Score          float64         `json:"score"`
	// ScoreOp selects how the merge-safe UPSERT combines Score with whatever
	// is already stored for this item_key (see the scoreOp* consts above).
	// A blanket GREATEST(EXCLUDED.score, knowledge_home_items.score) can
	// only ever ratchet a score up, which made HomeItemOpened's suppressed
	// 0.1 score unreachable once any higher score had ever been written.
	ScoreOp           string     `json:"score_op"`
	FreshnessAt       *time.Time `json:"freshness_at"`
	PublishedAt       *time.Time `json:"published_at"`
	LastInteractedAt  *time.Time `json:"last_interacted_at"`
	GeneratedAt       time.Time  `json:"generated_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DismissedAt       *time.Time `json:"dismissed_at"`
	ProjectionVersion int        `json:"projection_version"`
	SummaryState      string     `json:"summary_state"`
	SupersedeState    string     `json:"supersede_state"`
	SupersededAt      *time.Time `json:"superseded_at"`
	PreviousRefJSON   string     `json:"previous_ref_json"`
	URL               string     `json:"url"`
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
	// LastEventSeq is the folding event's own knowledge_events.event_seq.
	// The counters above are additive deltas, so sovereign_db's merge-safe
	// UPSERT needs a monotonic discriminator to tell "already folded" from
	// "another producer's clock is ahead" and applies the DO UPDATE only
	// above the row's stored high-water mark. UpdatedAt cannot serve: it is
	// the event's OccurredAt, stamped by whichever producer emitted it. The
	// driver rejects a payload that omits this rather than falling back, so
	// every digestWrite literal must set it.
	LastEventSeq int64 `json:"last_event_seq"`
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

// snoozeRecallCandidateWrite/dismissRecallCandidateWrite mirror the exact
// json.RawMessage unmarshal targets sovereign_db.Repository.
// SnoozeRecallCandidate/DismissRecallCandidate already expose (see
// repository.go) — the same write-through methods
// recall_snooze_usecase/recall_dismiss_usecase call directly from
// alt-backend, so replaying RecallSnoozed/RecallDismissed on reproject
// reaches the identical UPDATE.
type snoozeRecallCandidateWrite struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	Until      string `json:"until"`
	OccurredAt string `json:"occurred_at"`
}

type dismissRecallCandidateWrite struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	OccurredAt string `json:"occurred_at"`
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
	AddedCodes       []string `json:"added_codes"`
	PreviousWhyCodes []string `json:"previous_why_codes"`
}

// recallSnoozedPayload/recallDismissedPayload mirror the shapes
// alt-backend/app/orchestrator/usecase/recall_snooze_usecase and
// recall_dismiss_usecase marshal onto the event's payload column.
type recallSnoozedPayload struct {
	ItemKey      string `json:"item_key"`
	SnoozedUntil string `json:"snoozed_until"`
}

type recallDismissedPayload struct {
	ItemKey string `json:"item_key"`
}
