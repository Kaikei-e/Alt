package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── recall snooze/dismiss folds ──
//
// recall_candidate_view is a disposable projection (immutable-design-guard),
// so a TRUNCATE + full reproject replay must reach the same snoozed/dismissed
// state alt-backend's write-through usecases already applied directly. Unlike
// the recall_candidate side effect on HomeItemOpened (which a later open
// re-derives, and which is therefore non-fatal), the write here IS the
// event's entire purpose, so a repository failure is a hard-fail: it stops
// the batch rather than silently advancing the checkpoint past a lost
// snooze/dismiss.

// buildSnoozeRecallCandidateWrite is a pure builder that creates snoozeRecallCandidateWrite from RecallSnoozed.
func buildSnoozeRecallCandidateWrite(evt sovereign_db.KnowledgeEvent) (snoozeRecallCandidateWrite, error) {
	var payload recallSnoozedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return snoozeRecallCandidateWrite{}, fmt.Errorf("unmarshal RecallSnoozed payload: %w", err)
	}
	if payload.ItemKey == "" {
		return snoozeRecallCandidateWrite{}, fmt.Errorf("RecallSnoozed payload missing item_key")
	}

	write := snoozeRecallCandidateWrite{
		UserID:     resolveUserID(evt).String(),
		ItemKey:    payload.ItemKey,
		Until:      payload.SnoozedUntil,
		OccurredAt: evt.OccurredAt.Format(time.RFC3339Nano),
	}
	return write, nil
}

func (p *Projector) foldRecallSnoozed(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	write, err := buildSnoozeRecallCandidateWrite(evt)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(write)
	if err != nil {
		return fmt.Errorf("marshal RecallSnoozed payload: %w", err)
	}
	if err := p.repo.SnoozeRecallCandidate(ctx, raw); err != nil {
		return fmt.Errorf("snooze recall candidate: %w", err)
	}
	return nil
}

// buildDismissRecallCandidateWrite is a pure builder that creates dismissRecallCandidateWrite from RecallDismissed.
func buildDismissRecallCandidateWrite(evt sovereign_db.KnowledgeEvent) (dismissRecallCandidateWrite, error) {
	var payload recallDismissedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return dismissRecallCandidateWrite{}, fmt.Errorf("unmarshal RecallDismissed payload: %w", err)
	}
	if payload.ItemKey == "" {
		return dismissRecallCandidateWrite{}, fmt.Errorf("RecallDismissed payload missing item_key")
	}

	write := dismissRecallCandidateWrite{
		UserID:     resolveUserID(evt).String(),
		ItemKey:    payload.ItemKey,
		OccurredAt: evt.OccurredAt.Format(time.RFC3339Nano),
	}
	return write, nil
}

func (p *Projector) foldRecallDismissed(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	write, err := buildDismissRecallCandidateWrite(evt)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(write)
	if err != nil {
		return fmt.Errorf("marshal RecallDismissed payload: %w", err)
	}
	if err := p.repo.DismissRecallCandidate(ctx, raw); err != nil {
		return fmt.Errorf("dismiss recall candidate: %w", err)
	}
	return nil
}
