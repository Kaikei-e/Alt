import { describe, it, expect } from "vitest";
import { useFeatureFlags } from "./useFeatureFlags.svelte.ts";

describe("useFeatureFlags", () => {
	it("starts with empty flags", () => {
		const ff = useFeatureFlags();
		expect(ff.flags).toEqual([]);
		expect(ff.knowledgeHomeEnabled).toBe(false);
		expect(ff.trackingEnabled).toBe(false);
		expect(ff.projectionV2Enabled).toBe(false);
	});

	it("sets flags and reports enabled state", () => {
		const ff = useFeatureFlags();
		ff.setFlags([
			{ name: "enable_knowledge_home_page", enabled: true },
			{ name: "enable_knowledge_home_tracking", enabled: false },
			{ name: "enable_knowledge_home_projection_v2", enabled: true },
		]);
		expect(ff.knowledgeHomeEnabled).toBe(true);
		expect(ff.trackingEnabled).toBe(false);
		expect(ff.projectionV2Enabled).toBe(true);
	});

	it("isEnabled checks arbitrary flag names", () => {
		const ff = useFeatureFlags();
		ff.setFlags([{ name: "custom_flag", enabled: true }]);
		expect(ff.isEnabled("custom_flag")).toBe(true);
		expect(ff.isEnabled("unknown_flag")).toBe(false);
	});
});
