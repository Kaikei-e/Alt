/**
 * Canonical english labels for LoopPriority (ADR-000831 §13).
 * Used for `aria-description` on foreground/mid-context/deep-focus entries so
 * screen readers can relay urgency without relying on depth/saturation cues.
 *
 * i18n-ready: replace this map with a translation dictionary when locale support
 * is introduced. Keep the english default alongside so accessibility never
 * regresses when a translation is missing.
 */

import type { LoopPriorityName } from "$lib/connect/knowledge_loop";

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
