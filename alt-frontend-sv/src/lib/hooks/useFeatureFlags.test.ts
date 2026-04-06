import { describe, it, expect } from "vitest";
import { useFeatureFlags } from "./useFeatureFlags.svelte.ts";

describe("useFeatureFlags", () => {
	it("always returns true for all flags", () => {
		const ff = useFeatureFlags();
		expect(ff.flags).toEqual([]);
		expect(ff.knowledgeHomeEnabled).toBe(true);
		expect(ff.trackingEnabled).toBe(true);
		expect(ff.projectionV2Enabled).toBe(true);
	});

	it("isEnabled always returns true regardless of flag name", () => {
		const ff = useFeatureFlags();
		expect(ff.isEnabled("custom_flag")).toBe(true);
		expect(ff.isEnabled("unknown_flag")).toBe(true);
		expect(ff.isEnabled("enable_recall_rail")).toBe(true);
	});

	it("setFlags is a no-op", () => {
		const ff = useFeatureFlags();
		ff.setFlags([{ name: "any", enabled: false }]);
		expect(ff.isEnabled("any")).toBe(true);
	});
});
