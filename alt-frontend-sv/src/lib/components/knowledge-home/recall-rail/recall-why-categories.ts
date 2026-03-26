/**
 * Categorizes recall reasons for the RecallWhyPanel.
 * Categories: Revisit (re-engagement), Connection (cross-reference), Completion (unfinished).
 */

import type { RecallReasonData } from "$lib/connect/knowledge_home";

export interface RecallWhyGroup {
	key: string;
	label: string;
	items: Array<{ reason: RecallReasonData; displayLabel: string }>;
}

export function categorizeRecallReasons(
	_reasons: RecallReasonData[],
): RecallWhyGroup[] {
	throw new Error("not implemented");
}
