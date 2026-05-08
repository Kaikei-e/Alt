/**
 * Maps backend `recent_action_labels` (past-tense lowercase, e.g. "opened",
 * "compared") to user-facing CTA-register labels (e.g. "Open", "Compare").
 *
 * The mapping is intentionally a 1:1 lookup — labels surfaced on the
 * Continue card should read the same as the buttons that produced them, so
 * the user can connect "I clicked Compare → Compare appears here". Unknown
 * labels round-trip in title-case so a future backend addition (e.g.
 * "exported") still reads sensibly without a coordinated UI release.
 */

const LABEL_MAP: Record<string, string> = {
	opened: "Open",
	asked: "Ask",
	saved: "Save",
	compared: "Compare",
	revisited: "Revisit",
	snoozed: "Snooze",
	opened_recap: "Open Recap",
};

export function formatRecentActionLabel(raw: string): string {
	const normalized = raw?.trim().toLowerCase();
	if (!normalized) return "";
	const known = LABEL_MAP[normalized];
	if (known) return known;
	return normalized.charAt(0).toUpperCase() + normalized.slice(1);
}
