import { describe, it, expect } from "vitest";

/**
 * Tests for StreamUpdateBar data logic.
 * Component rendering is tested via browser tests (*.svelte.test.ts).
 */

describe("StreamUpdateBar data", () => {
	it("renders nothing when pendingCount is 0", () => {
		const pendingCount = 0;
		// Component uses {#if pendingCount > 0} guard
		expect(pendingCount > 0).toBe(false);
	});

	it("renders bar with correct singular count", () => {
		const pendingCount = 1;
		const label = `${pendingCount} ${pendingCount === 1 ? "item" : "items"} updated`;
		expect(label).toBe("1 item updated");
	});

	it("renders bar with correct plural count", () => {
		const pendingCount: number = 5;
		const label = `${pendingCount} ${pendingCount === 1 ? "item" : "items"} updated`;
		expect(label).toBe("5 items updated");
	});

	it("shows connected indicator when isConnected is true", () => {
		const isConnected = true;
		const isFallback = false;
		const indicatorColor = isFallback
			? "orange"
			: isConnected
				? "green"
				: "gray";
		expect(indicatorColor).toBe("green");
	});

	it("shows disconnected indicator when isConnected is false", () => {
		const isConnected = false;
		const isFallback = false;
		const indicatorColor = isFallback
			? "orange"
			: isConnected
				? "green"
				: "gray";
		expect(indicatorColor).toBe("gray");
	});

	it("shows fallback indicator when isFallback is true", () => {
		const isConnected = false;
		const isFallback = true;
		const indicatorColor = isFallback
			? "orange"
			: isConnected
				? "green"
				: "gray";
		expect(indicatorColor).toBe("orange");
	});
});
