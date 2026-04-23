import type { LoopStageName } from "$lib/connect/knowledge_loop";

// ADR-000831 §7 state machine allowlist. Any pair not in this set is rejected.
// Same-stage transitions (observe → observe, etc.) are intentionally omitted.
const ALLOWED: ReadonlySet<`${LoopStageName}->${LoopStageName}`> = new Set([
	"observe->orient",
	"observe->decide",
	"orient->decide",
	"decide->act",
	"act->observe",
]);

export function canTransition(from: LoopStageName, to: LoopStageName): boolean {
	return ALLOWED.has(`${from}->${to}`);
}

export function transitionReason(
	from: LoopStageName,
	to: LoopStageName,
): string {
	if (canTransition(from, to)) return "";
	return "Not available from this stage.";
}
