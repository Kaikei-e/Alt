import { describe, expect, it } from "vitest";

import { formatRecentActionLabel } from "./recent-action-label-map";

describe("formatRecentActionLabel", () => {
	it.each([
		["opened", "Open"],
		["asked", "Ask"],
		["saved", "Save"],
		["compared", "Compare"],
		["revisited", "Revisit"],
		["snoozed", "Snooze"],
		["opened_recap", "Open Recap"],
	])("maps %s to %s (matches CTA wording)", (raw, expected) => {
		expect(formatRecentActionLabel(raw)).toBe(expected);
	});

	it("title-cases unknown labels so a future backend addition still reads sensibly", () => {
		expect(formatRecentActionLabel("exported")).toBe("Exported");
	});

	it("returns empty string for empty input", () => {
		expect(formatRecentActionLabel("")).toBe("");
		expect(formatRecentActionLabel("   ")).toBe("");
	});

	it("is case-insensitive on the input", () => {
		expect(formatRecentActionLabel("COMPARED")).toBe("Compare");
		expect(formatRecentActionLabel("Compared")).toBe("Compare");
	});

	it("is deterministic across repeated invocations", () => {
		const first = formatRecentActionLabel("compared");
		for (let i = 0; i < 5; i++) {
			expect(formatRecentActionLabel("compared")).toBe(first);
		}
	});
});
