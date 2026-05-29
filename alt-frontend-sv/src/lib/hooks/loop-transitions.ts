import type { LoopStageName } from "$lib/connect/knowledge_loop";

// ADR-000831 §7 cross-stage allowlist. Mirrors `allowed_edges` in
// proto/alt/knowledge/loop/v1/loop_transition_policy.json. Conformance is
// pinned by loop-transitions.policy.test.ts.
const ALLOWED: ReadonlySet<`${LoopStageName}->${LoopStageName}`> = new Set([
	"observe->orient",
	"observe->decide",
	// Boyd implicit guidance & control: a trained reader commits straight to
	// Act from Observe or Orient without walking the explicit Decide step.
	"observe->act",
	"orient->observe",
	"orient->decide",
	"orient->act",
	"decide->act",
	"act->observe",
]);

// ADR-000914 same-stage trigger enumeration. Mirrors `same_stage_triggers`
// in loop_transition_policy.json — the BE classifier accepts these for
// from == to. Same-stage with any other trigger stays rejected so a stray
// "user_tap" cannot smuggle an idempotent transition past the policy.
export const SAME_STAGE_TRIGGERS = [
	"defer",
	"recheck",
	"archive",
	"mark_reviewed",
	"compare",
	"internalize",
	"intent_signal",
] as const;

export type SameStageTrigger = (typeof SAME_STAGE_TRIGGERS)[number];

export type TransitionTrigger =
	| SameStageTrigger
	| "user_tap"
	| "dwell"
	| "keyboard"
	| "programmatic";

const SAME_STAGE_TRIGGER_SET: ReadonlySet<string> = new Set(
	SAME_STAGE_TRIGGERS,
);

export function canTransition(
	from: LoopStageName,
	to: LoopStageName,
	trigger?: TransitionTrigger,
): boolean {
	if (from === to) {
		return trigger !== undefined && SAME_STAGE_TRIGGER_SET.has(trigger);
	}
	return ALLOWED.has(`${from}->${to}`);
}

export function transitionReason(
	from: LoopStageName,
	to: LoopStageName,
	trigger?: TransitionTrigger,
): string {
	if (canTransition(from, to, trigger)) return "";
	return "Not available from this stage.";
}
