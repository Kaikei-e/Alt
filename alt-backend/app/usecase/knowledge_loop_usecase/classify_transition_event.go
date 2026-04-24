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
)

// ClassifyTransitionEvent derives the canonical knowledge_events.event_type string
// from (from_stage, to_stage, trigger). It rejects forbidden transitions listed
// in ADR-000831 §7 (forbidden set) so callers cannot append nonsensical events
// that would corrupt session-state projection on replay.
//
// The function is pure and has no hidden time/state dependencies, which makes
// it reproject-safe: projector replay derives the same event classification as
// the original append path.
func ClassifyTransitionEvent(fromStage, toStage, trigger string) (string, error) {
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
