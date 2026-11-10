import { describe, expect, it } from "vitest";
import { boxesOverlap, type LabelBox, layoutLabels } from "./labelLayout.js";

describe("labelLayout", () => {
	it("rejects an overlapping lower-priority box", () => {
		const boxes: LabelBox[] = [
			{ x: 10, y: 10, w: 60, h: 18 },
			{ x: 30, y: 14, w: 60, h: 18 },
		];
		expect(layoutLabels(boxes)).toEqual([0]);
	});

	it("accepts all non-overlapping boxes", () => {
		const boxes: LabelBox[] = [
			{ x: 0, y: 0, w: 50, h: 16 },
			{ x: 60, y: 0, w: 50, h: 16 },
			{ x: 0, y: 30, w: 50, h: 16 },
		];
		expect(layoutLabels(boxes)).toEqual([0, 1, 2]);
	});

	it("respects priority order when placing greedy candidates", () => {
		const boxes: LabelBox[] = [
			{ x: 0, y: 0, w: 50, h: 20 },
			{ x: 40, y: 0, w: 50, h: 20 }, // overlaps 0 -> rejected
			{ x: 55, y: 0, w: 50, h: 20 }, // overlaps 1, but 1 was rejected -> accepted
		];
		expect(layoutLabels(boxes)).toEqual([0, 2]);
	});

	it("always accepts forced indices (selected/hovered) even when they overlap", () => {
		const boxes: LabelBox[] = [
			{ x: 20, y: 20, w: 80, h: 20, forced: true },
			{ x: 25, y: 22, w: 80, h: 20, forced: true },
			{ x: 30, y: 24, w: 80, h: 20 },
		];
		expect(layoutLabels(boxes, { forcedIndices: [0, 1] })).toEqual([0, 1]);
	});

	it("places 1000 random boxes without error and with zero pairwise overlap among accepted boxes", () => {
		let seed = 123456789;
		const rand = () => {
			seed = (seed * 1664525 + 1013904223) >>> 0;
			return seed / 0x100000000;
		};

		const boxes: LabelBox[] = Array.from({ length: 1000 }, () => ({
			x: rand() * 1920,
			y: rand() * 1080,
			w: 30 + rand() * 90,
			h: 12 + rand() * 8,
		}));

		const accepted = layoutLabels(boxes);
		expect(accepted.length).toBeGreaterThan(0);

		for (let i = 0; i < accepted.length; i++) {
			const boxA = boxes[accepted[i] as number] as LabelBox;
			for (let j = i + 1; j < accepted.length; j++) {
				const boxB = boxes[accepted[j] as number] as LabelBox;
				expect(boxesOverlap(boxA, boxB)).toBe(false);
			}
		}
	});
});
