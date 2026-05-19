/**
 * Canonical english labels for LoopPriority (ADR-000831 §13).
 * Used for `aria-description` on foreground/mid-context/deep-focus entries so
 * screen readers can relay urgency without relying on depth/saturation cues.
 *
 * English is the default per Alt's UI text policy. The Japanese dictionary
 * below is required by canonical contract §13. A locale-aware helper
 * (`loopPriorityAriaLabelFor` / `loopPriorityLabelFor`) selects the right map.
 * Existing call sites continue to use the english record directly; future
 * locale wiring can route through the helper without breaking accessibility.
 */

import type { LoopPriorityName } from "$lib/connect/knowledge_loop";

export type LoopPriorityLocale = "en" | "ja";

export const loopPriorityAriaLabel: Record<LoopPriorityName, string> = {
	critical: "Critical entry",
	continuing: "Continuing entry",
	confirm: "Change to confirm",
	reference: "Reference entry",
};

/**
 * Human-readable single-word label matched to LoopPriority. Shown as the
 * monospace meta badge on tiles inside the Continue / Changed / Review planes.
 */
export const loopPriorityLabel: Record<LoopPriorityName, string> = {
	critical: "Critical",
	continuing: "Continuing",
	confirm: "Confirm",
	reference: "Reference",
};

/**
 * Japanese aria-description dictionary per canonical contract §13. Required
 * because depth on tiles is not perceivable for screen-reader users; the
 * priority label is how urgency is conveyed in any locale.
 */
export const loopPriorityAriaLabelJa: Record<LoopPriorityName, string> = {
	critical: "最重要のエントリ",
	continuing: "継続中のエントリ",
	confirm: "変更確認が必要なエントリ",
	reference: "参照用のエントリ",
};

export const loopPriorityLabelJa: Record<LoopPriorityName, string> = {
	critical: "最重要",
	continuing: "継続中",
	confirm: "変更確認推奨",
	reference: "参照のみ",
};

export function loopPriorityAriaLabelFor(
	locale: LoopPriorityLocale,
	priority: LoopPriorityName,
): string {
	return locale === "ja"
		? loopPriorityAriaLabelJa[priority]
		: loopPriorityAriaLabel[priority];
}

export function loopPriorityLabelFor(
	locale: LoopPriorityLocale,
	priority: LoopPriorityName,
): string {
	return locale === "ja"
		? loopPriorityLabelJa[priority]
		: loopPriorityLabel[priority];
}
