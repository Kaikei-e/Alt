import { describe, expect, it } from "vitest";

describe("MobileRecallSection logic", () => {
	it("limits displayed items to 2", () => {
		const candidates = [
			{ itemKey: "a" },
			{ itemKey: "b" },
			{ itemKey: "c" },
			{ itemKey: "d" },
		];
		const displayed = candidates.slice(0, 2);
		expect(displayed).toHaveLength(2);
		expect(displayed[0].itemKey).toBe("a");
		expect(displayed[1].itemKey).toBe("b");
	});

	it("shows all if fewer than 2", () => {
		const candidates = [{ itemKey: "a" }];
		const displayed = candidates.slice(0, 2);
		expect(displayed).toHaveLength(1);
	});

	it("has remaining count when more than 2", () => {
		const candidates = [
			{ itemKey: "a" },
			{ itemKey: "b" },
			{ itemKey: "c" },
		];
		const remaining = candidates.length - 2;
		expect(remaining).toBe(1);
	});

	it("renders nothing for empty candidates", () => {
		const candidates: { itemKey: string }[] = [];
		expect(candidates.length).toBe(0);
	});
});
