import { describe, expect, it } from "vitest";
import { groupFootprintsByDay } from "./trail-grouping";
import type { FootprintData } from "$lib/connect/knowledge_trail";

function fp(key: string, occurredAt: string): FootprintData {
	return {
		footprintKey: key,
		verb: "read",
		itemKey: `article:${key}`,
		title: key,
		excerpt: "",
		tags: [],
		note: "",
		occurredAt,
		wear: "thin",
	};
}

// Dates are constructed in LOCAL time (and serialised via toISOString) so the
// grouping — which keys by local day — is asserted identically regardless of the
// test runner's timezone.
function localISO(y: number, mZeroBased: number, d: number, h: number): string {
	return new Date(y, mZeroBased, d, h, 0, 0).toISOString();
}

describe("groupFootprintsByDay", () => {
	const now = new Date(2026, 5, 10, 15, 0, 0); // June 10 2026, 15:00 local

	it("labels today and yesterday relative to the injected now", () => {
		const groups = groupFootprintsByDay(
			[
				fp("a", localISO(2026, 5, 10, 9)),
				fp("b", localISO(2026, 5, 9, 20)),
			],
			now,
		);
		expect(groups).toHaveLength(2);
		expect(groups[0].label).toBe("Today");
		expect(groups[1].label).toBe("Yesterday");
	});

	it("keeps reverse-chronological footprints in the same day bucket in order", () => {
		const groups = groupFootprintsByDay(
			[
				fp("a", localISO(2026, 5, 10, 13)),
				fp("b", localISO(2026, 5, 10, 11)),
			],
			now,
		);
		expect(groups).toHaveLength(1);
		expect(groups[0].footprints.map((f) => f.footprintKey)).toEqual(["a", "b"]);
	});

	it("buckets unparseable timestamps under Earlier", () => {
		const groups = groupFootprintsByDay([fp("x", "not-a-date")], now);
		expect(groups[0].label).toBe("Earlier");
	});
});
