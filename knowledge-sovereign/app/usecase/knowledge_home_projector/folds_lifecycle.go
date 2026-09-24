package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── fold functions ──
//
// Each fold derives every business fact (timestamps, scores) from the
// event's own OccurredAt/payload — never wall clock, never a read model —
// so replaying the same log reproduces identical rows (reproject-safe).

// ── HomeItemOpened ──

// buildHomeItemOpenedWrites is a pure builder that calculates the homeItemWrite,
// clearSupersedeWrite, and recallCandidateWrite structs from HomeItemOpened.
func buildHomeItemOpenedWrites(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, clearSupersedeWrite, recallCandidateWrite, error) {
	var payload homeItemOpenedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, clearSupersedeWrite{}, recallCandidateWrite{}, fmt.Errorf("unmarshal HomeItemOpened payload: %w", err)
	}

	occurredAt := evt.OccurredAt
	userID := resolveUserID(evt)
	item := homeItemWrite{
		UserID:            userID,
		TenantID:          evt.TenantID,
		ItemKey:           payload.ItemKey,
		ItemType:          itemTypeArticle,
		WhyReasons:        []whyReasonWire{{Code: whyNewUnread}},
		Score:             0.1,        // suppressed: opening an item lowers its resurfacing score
		ScoreOp:           scoreOpSet, // must be authoritative — a floor merge could never lower the score
		LastInteractedAt:  &occurredAt,
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: version,
	}

	clear := clearSupersedeWrite{
		UserID:            userID.String(),
		ItemKey:           payload.ItemKey,
		ProjectionVersion: version,
	}

	// Recall candidate: eligible 1h after the event's own time.
	eligibleAt := occurredAt.Add(1 * time.Hour)
	candidate := recallCandidateWrite{
		UserID:            userID,
		ItemKey:           payload.ItemKey,
		RecallScore:       0.5,
		Reasons:           []recallReasonWire{{Type: recallReasonOpenedNotRevisited, Description: "Opened but not revisited"}},
		FirstEligibleAt:   &eligibleAt,
		NextSuggestAt:     &eligibleAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: version,
	}
	return item, clear, candidate, nil
}

func (p *Projector) foldHomeItemOpened(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, clear, candidate, err := buildHomeItemOpenedWrites(evt, version)
	if err != nil {
		return err
	}
	if err := p.upsertHomeItem(ctx, item); err != nil {
		return err
	}

	// Clear supersede state on open (acknowledgement). Non-fatal.
	if err := p.clearSupersedeState(ctx, clear); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: clear supersede state failed on open",
			slog.String("event_id", evt.EventID.String()), slog.String("item_key", clear.ItemKey), slog.Any("error", err))
	}

	// Recall candidate: eligible 1h after the event's own time. Non-fatal.
	if err := p.upsertRecallCandidate(ctx, candidate); err != nil {
		p.logger.WarnContext(ctx, "knowledge_home_projector: recall candidate upsert failed",
			slog.String("event_id", evt.EventID.String()), slog.Any("error", err))
	}
	return nil
}

// ── HomeItemDismissed ──

// buildDismissWrite is a pure builder that extracts the dismissWrite struct from HomeItemDismissed.
func buildDismissWrite(evt sovereign_db.KnowledgeEvent, version int) (dismissWrite, error) {
	var payload homeItemDismissedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return dismissWrite{}, fmt.Errorf("unmarshal HomeItemDismissed payload: %w", err)
	}
	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = evt.AggregateID
	}
	if itemKey == "" {
		return dismissWrite{}, fmt.Errorf("home item dismiss payload missing item_key")
	}

	dismiss := dismissWrite{
		UserID:            resolveUserID(evt).String(),
		ItemKey:           itemKey,
		ProjectionVersion: version,
		// Business fact from the event only — knowledge_events.occurred_at is
		// NOT NULL, so there is no wall-clock fallback here (reproject-safe).
		DismissedAt: evt.OccurredAt.Format(time.RFC3339Nano),
	}
	return dismiss, nil
}

func (p *Projector) foldHomeItemDismissed(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	dismiss, err := buildDismissWrite(evt, version)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(dismiss)
	if err != nil {
		return fmt.Errorf("marshal HomeItemDismissed payload: %w", err)
	}
	if err := p.repo.DismissKnowledgeHomeItem(ctx, raw); err != nil {
		// A dismiss whose target row does not exist is a benign no-op fold,
		// not a failure: the handler only validates that item_key is
		// non-empty, so a client can dismiss a key that never produced a
		// knowledge_home_items row at this projection version. ADR-000473
		// declared that condition non-fatal by design and alt-backend's
		// write-through already logs and swallows it — only this fold treated
		// it as fatal, which made a single such event a poison pill that
		// wedged the checkpoint (and every user's Knowledge Home) forever.
		//
		// This is narrowly the not-found sentinel, NOT general event
		// skipping: ADR-000456 explicitly rejected "skip the failing event
		// and advance the checkpoint" as a policy, so every other error below
		// still stops the batch. Nothing is dropped either — the event stays
		// in the append-only log and a reproject re-folds it to the same
		// no-op, so the read model remains reproject-safe.
		if errors.Is(err, sovereign_db.ErrDismissTargetNotFound) {
			p.logger.WarnContext(ctx, "knowledge_home_projector: dismiss target row not found, folding as no-op",
				slog.String("event_id", evt.EventID.String()), slog.String("item_key", dismiss.ItemKey),
				slog.Int("projection_version", version))
			return nil
		}
		return fmt.Errorf("dismiss knowledge_home_items: %w", err)
	}
	return nil
}

// ── supersede folds ──

// buildSummarySupersededWrite is a pure builder that creates the homeItemWrite for SummarySuperseded.
func buildSummarySupersededWrite(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, error) {
	var payload summarySupersededPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, fmt.Errorf("unmarshal SummarySuperseded payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("parse article_id: %w", err)
	}
	prevRef, err := json.Marshal(map[string]string{"previous_summary_excerpt": payload.PreviousSummaryExcerpt})
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("marshal previous_summary_excerpt ref: %w", err)
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
		ProjectionVersion: version,
	}
	return item, nil
}

func (p *Projector) foldSummarySuperseded(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, err := buildSummarySupersededWrite(evt, version)
	if err != nil {
		return err
	}
	return p.upsertHomeItem(ctx, item)
}

// buildTagSetSupersededWrite is a pure builder that creates the homeItemWrite for TagSetSuperseded.
func buildTagSetSupersededWrite(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, error) {
	var payload tagSetSupersededPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, fmt.Errorf("unmarshal TagSetSuperseded payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("parse article_id: %w", err)
	}
	prevRef, err := json.Marshal(map[string][]string{"previous_tags": payload.PreviousTags})
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("marshal previous_tags ref: %w", err)
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
		ProjectionVersion: version,
	}
	return item, nil
}

func (p *Projector) foldTagSetSuperseded(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, err := buildTagSetSupersededWrite(evt, version)
	if err != nil {
		return err
	}
	return p.upsertHomeItem(ctx, item)
}

// buildReasonMergedWrite is a pure builder that creates the homeItemWrite for ReasonMerged.
func buildReasonMergedWrite(evt sovereign_db.KnowledgeEvent, version int) (homeItemWrite, error) {
	var payload reasonMergedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return homeItemWrite{}, fmt.Errorf("unmarshal ReasonMerged payload: %w", err)
	}
	articleID, err := uuid.Parse(payload.ArticleID)
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("parse article_id: %w", err)
	}
	itemKey := payload.ItemKey
	if itemKey == "" {
		itemKey = fmt.Sprintf("article:%s", articleID)
	}
	prevRef, err := json.Marshal(map[string][]string{"previous_why_codes": payload.PreviousWhyCodes})
	if err != nil {
		return homeItemWrite{}, fmt.Errorf("marshal previous_why_codes ref: %w", err)
	}

	// why_reasons is deliberately built even when empty (an explicit empty
	// slice, not nil — see foldSummarySuperseded) rather than left as the
	// homeItemWrite zero value: the merge-safe UPSERT's why_json CASE
	// (repository.go) already treats an empty '[]' as "add nothing", so an
	// event that adds no codes correctly no-ops while one that does gets
	// unioned into the stored why_json instead of silently dropped.
	whyReasons := make([]whyReasonWire, 0, len(payload.AddedCodes))
	for _, code := range payload.AddedCodes {
		whyReasons = append(whyReasons, whyReasonWire{Code: code})
	}

	occurredAt := evt.OccurredAt
	item := homeItemWrite{
		UserID:            resolveUserID(evt),
		TenantID:          evt.TenantID,
		ItemKey:           itemKey,
		ItemType:          itemTypeArticle,
		WhyReasons:        whyReasons,
		SupersedeState:    supersedeReasonUpdated,
		SupersededAt:      &occurredAt,
		PreviousRefJSON:   string(prevRef),
		GeneratedAt:       occurredAt,
		UpdatedAt:         occurredAt,
		ProjectionVersion: version,
	}
	return item, nil
}

func (p *Projector) foldReasonMerged(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	item, err := buildReasonMergedWrite(evt, version)
	if err != nil {
		return err
	}
	return p.upsertHomeItem(ctx, item)
}
