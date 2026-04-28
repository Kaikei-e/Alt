package knowledge_loop_usecase

import (
	"alt/domain"
	"fmt"
)

// Proto enum names for LoopStage. Kept as strings so this package does not
// depend on the generated proto types; callers pass enum.String() values.
const (
	stageObserve = "LOOP_STAGE_OBSERVE"
	stageOrient  = "LOOP_STAGE_ORIENT"
	stageDecide  = "LOOP_STAGE_DECIDE"
	stageAct     = "LOOP_STAGE_ACT"

	triggerDwell    = "TRANSITION_TRIGGER_DWELL"
	triggerUserTap  = "TRANSITION_TRIGGER_USER_TAP"
	triggerKeyboard = "TRANSITION_TRIGGER_KEYBOARD"
	triggerProgram  = "TRANSITION_TRIGGER_PROGRAMMATIC"
	// triggerDefer routes a "soft dismiss / snooze" intent to the canonical
	// KnowledgeLoopDeferred event (canonical contract §8.2). It is the only
	// trigger that allows from_stage == to_stage.
	triggerDefer = "TRANSITION_TRIGGER_DEFER"

	// Review-lane triggers (fb.md §F: Review re-evaluation engine). Each is
	// same-stage like triggerDefer — the OODA stage doesn't move; only
	// dismiss_state does. Distinct from Deferred because the user is making
	// an explicit re-evaluation decision, not a "look at this later" snooze.
	triggerRecheck      = "TRANSITION_TRIGGER_RECHECK"
	triggerArchive      = "TRANSITION_TRIGGER_ARCHIVE"
	triggerMarkReviewed = "TRANSITION_TRIGGER_MARK_REVIEWED"
)

// isReviewActionTrigger reports whether the trigger is one of the three
// Review-lane intents that route to KnowledgeLoopReviewed. They share the
// same dispatch shape as triggerDefer (same-stage, OODA-canonical) so the
// classifier handles them through one branch.
func isReviewActionTrigger(t string) bool {
	switch t {
	case triggerRecheck, triggerArchive, triggerMarkReviewed:
		return true
	}
	return false
}

// ClassifyTransitionEvent derives the canonical knowledge_events.event_type string
// from (from_stage, to_stage, trigger). It rejects forbidden transitions listed
// in ADR-000831 §7 (forbidden set) so callers cannot append nonsensical events
// that would corrupt session-state projection on replay.
//
// The function is pure and has no hidden time/state dependencies, which makes
// it reproject-safe: projector replay derives the same event classification as
// the original append path.
func ClassifyTransitionEvent(fromStage, toStage, trigger string) (string, error) {
	// DEFER trigger short-circuits the OODA stage matrix. It is a passive
	// "soft dismiss / snooze" intent (canonical contract §8.2) that does not
	// move the user along the loop — it just records that the user wants this
	// entry out of view. Same-stage is required (and is the only place same-
	// stage transitions are allowed).
	if trigger == triggerDefer {
		if !isCanonicalStage(fromStage) || fromStage != toStage {
			return "", fmt.Errorf("%w: defer trigger requires from_stage == to_stage and a canonical OODA stage", ErrInvalidArgument)
		}
		return domain.EventKnowledgeLoopDeferred, nil
	}

	// Review-lane triggers (recheck / archive / mark_reviewed) all route to
	// KnowledgeLoopReviewed. The action distinction is the trigger value
	// itself, so the projector can patch dismiss_state appropriately while
	// replay stays deterministic. Same-stage requirement matches triggerDefer.
	if isReviewActionTrigger(trigger) {
		if !isCanonicalStage(fromStage) || fromStage != toStage {
			return "", fmt.Errorf("%w: review-lane trigger requires from_stage == to_stage and a canonical OODA stage", ErrInvalidArgument)
		}
		return domain.EventKnowledgeLoopReviewed, nil
	}

	// Forbidden transitions (reject before any allow-list check).
	if fromStage == stageObserve && toStage == stageAct {
		return "", fmt.Errorf("%w: observe->act not allowed", ErrInvalidArgument)
	}
	if fromStage == stageAct && toStage == stageAct {
		return "", fmt.Errorf("%w: act->act not allowed", ErrInvalidArgument)
	}
	// decide->observe is only legal via an explicit KnowledgeLoopReturned triggered by act->observe.
	if fromStage == stageDecide && toStage == stageObserve {
		return "", fmt.Errorf("%w: decide->observe not allowed", ErrInvalidArgument)
	}

	switch {
	case fromStage == stageObserve && toStage == stageOrient:
		if trigger == triggerDwell {
			return domain.EventKnowledgeLoopObserved, nil
		}
		return domain.EventKnowledgeLoopOriented, nil
	case fromStage == stageObserve && toStage == stageDecide:
		// Bypassing orient — classify as coarse Oriented to keep session state continuous.
		return domain.EventKnowledgeLoopOriented, nil
	case fromStage == stageOrient && toStage == stageDecide:
		return domain.EventKnowledgeLoopDecisionPresented, nil
	case fromStage == stageDecide && toStage == stageAct:
		return domain.EventKnowledgeLoopActed, nil
	case fromStage == stageAct && toStage == stageObserve:
		return domain.EventKnowledgeLoopReturned, nil
	case fromStage == stageOrient && toStage == stageObserve:
		// observe revisit from orient (allowed under review->orient->observe chain).
		return domain.EventKnowledgeLoopObserved, nil
	}

	return "", fmt.Errorf("%w: unrecognized stage tuple from=%s to=%s", ErrInvalidArgument, safeStageLabel(fromStage), safeStageLabel(toStage))
}

// safeStageLabel avoids echoing attacker-controlled raw strings into error text.
// Only the allow-listed proto enum names pass through unchanged.
func safeStageLabel(s string) string {
	switch s {
	case stageObserve, stageOrient, stageDecide, stageAct:
		return s
	default:
		return "UNSPECIFIED"
	}
}

// isCanonicalStage reports whether s is one of the four OODA stages. Used by
// the DEFER short-circuit to reject UNSPECIFIED / unknown stages without
// echoing the raw value.
func isCanonicalStage(s string) bool {
	switch s {
	case stageObserve, stageOrient, stageDecide, stageAct:
		return true
	}
	return false
}
