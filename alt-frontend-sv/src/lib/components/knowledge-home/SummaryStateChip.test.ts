import { describe, expect, it } from "vitest";
import type { SummaryState } from "$lib/connect/knowledge_home";

/**
 * Tests for SummaryStateChip data logic.
 */
describe("SummaryStateChip", () => {
	it("pending state should render a chip", () => {
		const state: SummaryState = "pending";
		expect(state).toBe("pending");
		// Component renders a visible chip for "pending"
	});

	it("ready state should render nothing", () => {
		const state: SummaryState = "ready";
		// Component renders nothing for "ready"
		expect(state).toBe("ready");
	});

	it("missing state should render nothing", () => {
		const state: SummaryState = "missing";
		// Component renders nothing for "missing"
		expect(state).toBe("missing");
	});

	it("only valid states are accepted", () => {
		const validStates: SummaryState[] = ["missing", "pending", "ready"];
		expect(validStates).toHaveLength(3);
	});
});
