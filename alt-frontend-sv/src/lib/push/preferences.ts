/**
 * Per-kind notification settings, and the rule for what a change to them means.
 *
 * A Web Push subscription is per device, and its mere existence is what puts a
 * device into the server's fan-out. So "which kinds are on" and "is this device
 * subscribed at all" are two different pieces of state that have to be kept
 * consistent — turning the last kind off has to remove the subscription, not
 * just clear four booleans on a row that keeps receiving.
 */

export const NOTIFICATION_KINDS = [
	"summary_ready",
	"acolyte_report_ready",
	"recap_ready",
	"today_entrance_ready",
] as const;

export type NotificationKind = (typeof NOTIFICATION_KINDS)[number];

export type Preferences = Record<NotificationKind, boolean>;

export type PreferenceAction = "subscribe" | "update" | "unsubscribe" | "none";

export function noPreferences(): Preferences {
	return {
		summary_ready: false,
		acolyte_report_ready: false,
		recap_ready: false,
		today_entrance_ready: false,
	};
}

export function anyEnabled(preferences: Preferences): boolean {
	return NOTIFICATION_KINDS.some((kind) => preferences[kind]);
}

export function decideAction(
	hasSubscription: boolean,
	next: Preferences,
): PreferenceAction {
	const wanted = anyEnabled(next);

	if (wanted) return hasSubscription ? "update" : "subscribe";
	return hasSubscription ? "unsubscribe" : "none";
}

/** UI copy. Deliberately factual: these describe events, not invitations. */
export const KIND_LABELS: Record<NotificationKind, string> = {
	summary_ready: "Summary finished",
	acolyte_report_ready: "Acolyte report finished",
	recap_ready: "Recap finished",
	today_entrance_ready: "Today's entrance is ready",
};

export const KIND_DESCRIPTIONS: Record<NotificationKind, string> = {
	summary_ready: "When a summary you asked for is done.",
	acolyte_report_ready: "When a report you asked for is done.",
	recap_ready: "When a recap you asked for is done.",
	today_entrance_ready: "Once each morning, if there is anything new.",
};
