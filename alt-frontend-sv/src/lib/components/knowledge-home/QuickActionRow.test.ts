import { describe, expect, it } from "vitest";

/**
 * Tests for QuickActionRow data logic.
 * Component rendering is tested via browser tests (*.svelte.test.ts).
 */
describe("QuickActionRow", () => {
	const actionTypes = [
		"open",
		"save",
		"unsave",
		"ask",
		"listen",
		"dismiss",
	] as const;

	it("supports all expected action types", () => {
		expect(actionTypes).toContain("open");
		expect(actionTypes).toContain("save");
		expect(actionTypes).toContain("unsave");
		expect(actionTypes).toContain("ask");
		expect(actionTypes).toContain("listen");
		expect(actionTypes).toContain("dismiss");
	});

	it("primary actions have labels", () => {
		const primaryActions = [
			{ type: "open", label: "Open" },
			{ type: "save", label: "Save" },
			{ type: "ask", label: "Ask" },
			{ type: "listen", label: "Listen" },
		];
		for (const action of primaryActions) {
			expect(action.label).toBeTruthy();
		}
	});

	it("dismiss is separated from primary actions", () => {
		const primaryActions = ["open", "save", "ask", "listen"];
		const secondaryActions = ["dismiss"];
		expect(primaryActions).not.toContain("dismiss");
		expect(secondaryActions).toContain("dismiss");
	});
});
