/**
 * Categorizes recall reasons for the RecallWhyPanel.
 * Categories: Revisit (re-engagement), Connection (cross-reference), Completion (unfinished).
 */

import type { RecallReasonData } from "$lib/connect/knowledge_home";
import { resolveRecallReason } from "./recall-reason-map";

export interface RecallWhyGroup {
	key: string;
	label: string;
	items: Array<{ reason: RecallReasonData; displayLabel: string }>;
}

const REVISIT_CODES = new Set(["opened_before_but_not_revisited"]);
const CONNECTION_CODES = new Set([
	"related_to_recent_search",
	"related_to_recent_augur_question",
	"tag_interest_overlap",
	"tag_interaction",
]);
const COMPLETION_CODES = new Set([
	"recap_context_unfinished",
	"pulse_followup_needed",
]);

const CATEGORY_ORDER = ["revisit", "connection", "completion", "other"];

const CATEGORY_LABELS: Record<string, string> = {
	revisit: "Revisit",
	connection: "Connection",
	completion: "Completion",
	other: "Other",
};

function categorize(code: string): string {
	if (REVISIT_CODES.has(code)) return "revisit";
	if (CONNECTION_CODES.has(code)) return "connection";
	if (COMPLETION_CODES.has(code)) return "completion";
	return "other";
}

export function categorizeRecallReasons(
	reasons: RecallReasonData[],
): RecallWhyGroup[] {
	if (reasons.length === 0) return [];

	const bucket = new Map<
		string,
		Array<{ reason: RecallReasonData; displayLabel: string }>
	>();

	for (const reason of reasons) {
		const cat = categorize(reason.type);
		const display = resolveRecallReason(reason.type, reason.description);
		const items = bucket.get(cat) ?? [];
		items.push({ reason, displayLabel: display.label });
		bucket.set(cat, items);
	}

	const groups: RecallWhyGroup[] = [];
	for (const key of CATEGORY_ORDER) {
		const items = bucket.get(key);
		if (items) {
			groups.push({ key, label: CATEGORY_LABELS[key], items });
		}
	}

	return groups;
}
