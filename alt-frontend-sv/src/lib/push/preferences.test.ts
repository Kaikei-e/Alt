import { describe, expect, it } from "vitest";

import {
	anyEnabled,
	decideAction,
	NOTIFICATION_KINDS,
	noPreferences,
	type Preferences,
} from "./preferences";

function prefs(overrides: Partial<Preferences> = {}): Preferences {
	return { ...noPreferences(), ...overrides };
}

describe("NOTIFICATION_KINDS", () => {
	it("covers exactly the four kinds the backend can send", () => {
		expect([...NOTIFICATION_KINDS]).toEqual([
			"summary_ready",
			"acolyte_report_ready",
			"recap_ready",
			"today_entrance_ready",
		]);
	});
});

describe("anyEnabled", () => {
	it("is false for a fresh preference set", () => {
		expect(anyEnabled(noPreferences())).toBe(false);
	});

	it("is true as soon as one kind is on", () => {
		expect(anyEnabled(prefs({ recap_ready: true }))).toBe(true);
	});
});

describe("decideAction", () => {
	it("subscribes when the first kind is turned on and no subscription exists", () => {
		expect(decideAction(false, prefs({ recap_ready: true }))).toBe("subscribe");
	});

	it("updates when a subscription already exists", () => {
		expect(
			decideAction(true, prefs({ recap_ready: true, summary_ready: true })),
		).toBe("update");
	});

	it("unsubscribes when the last kind is turned off", () => {
		// A device that wants nothing must leave the fan-out rather than sit in
		// it receiving pushes it will not display — Safari revokes the site's
		// permission for a push that shows no notification.
		expect(decideAction(true, noPreferences())).toBe("unsubscribe");
	});

	it("does nothing when there is no subscription and nothing is on", () => {
		expect(decideAction(false, noPreferences())).toBe("none");
	});
});
