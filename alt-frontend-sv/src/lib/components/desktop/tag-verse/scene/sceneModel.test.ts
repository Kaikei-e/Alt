import { describe, expect, it } from "vitest";
import type { TagCloudItem } from "#lib/connect/index.js";
import {
	buildSceneModel,
	computeCloudRadius,
	computeColorBucket,
	computeFogFactor,
	computeLabelFontSize,
	computeLabelOpacity,
	computeRadius,
	fibonacciSphere,
	fogWindow,
	hasServerPositions,
} from "./sceneModel.js";

function makeTag(
	tagName: string,
	articleCount: number,
	x = 0,
	y = 0,
	z = 0,
): TagCloudItem {
	return {
		tagName,
		articleCount,
		positionX: x,
		positionY: y,
		positionZ: z,
	};
}

describe("sceneModel", () => {
	it("maps colour buckets using t = (count / max)^0.5 at threshold boundaries 0.2, 0.4, 0.6, 0.8", () => {
		const max = 100;
		// t = sqrt(count / 100) => count = 100 * t^2
		// t = 0 -> 0, t = 0.19 -> 3.61, t = 0.2 -> 4, t = 0.39 -> 15.21, t = 0.4 -> 16,
		// t = 0.59 -> 34.81, t = 0.6 -> 36, t = 0.79 -> 62.41, t = 0.8 -> 64, t = 1.0 -> 100
		expect(computeColorBucket(0, max)).toBe(0);
		expect(computeColorBucket(3.61, max)).toBe(0);
		expect(computeColorBucket(4, max)).toBe(1);
		expect(computeColorBucket(15.21, max)).toBe(1);
		expect(computeColorBucket(16, max)).toBe(2);
		expect(computeColorBucket(34.81, max)).toBe(2);
		expect(computeColorBucket(36, max)).toBe(3);
		expect(computeColorBucket(62.41, max)).toBe(3);
		expect(computeColorBucket(64, max)).toBe(4);
		expect(computeColorBucket(100, max)).toBe(4);
	});

	it("computes node radius in [0.3, 3.0] with ^0.4 power scale", () => {
		expect(computeRadius(0, 100)).toBeCloseTo(0.3, 6);
		expect(computeRadius(100, 100)).toBeCloseTo(3.0, 6);
		const mid = computeRadius(25, 100);
		expect(mid).toBeCloseTo(0.3 + 0.25 ** 0.4 * 2.7, 6);
		expect(mid).toBeGreaterThan(0.3);
		expect(mid).toBeLessThan(3.0);
	});

	it("computes label font size in [11, 16] with ^0.4 power scale", () => {
		expect(computeLabelFontSize(0, 100)).toBeCloseTo(11, 6);
		expect(computeLabelFontSize(100, 100)).toBeCloseTo(16, 6);
		const mid = computeLabelFontSize(25, 100);
		expect(mid).toBeCloseTo(11 + 0.25 ** 0.4 * 5, 6);
		expect(mid).toBeGreaterThan(11);
		expect(mid).toBeLessThan(16);
	});

	it("generates count points on a Fibonacci sphere of the requested radius", () => {
		const count = 20;
		const radius = 42;
		const points = fibonacciSphere(count, radius);

		expect(points).toHaveLength(count);
		for (const [x, y, z] of points) {
			expect(Math.hypot(x, y, z)).toBeCloseTo(radius, 5);
		}
	});

	it("detects server positions only when at least one coordinate is non-zero", () => {
		expect(hasServerPositions([])).toBe(false);
		expect(
			hasServerPositions([makeTag("a", 5, 0, 0, 0), makeTag("b", 3, 0, 0, 0)]),
		).toBe(false);
		expect(
			hasServerPositions([makeTag("a", 5, 0, 0, 0), makeTag("b", 3, 0, 0, 1)]),
		).toBe(true);
	});

	it("preserves cloudRadius rules for both server positions and fallback sphere", () => {
		const fallbackSmall = [makeTag("a", 1), makeTag("b", 2)];
		expect(computeCloudRadius(fallbackSmall)).toBe(15);

		const fallbackMany = Array.from({ length: 100 }, (_, i) =>
			makeTag(`t${i}`, i + 1),
		);
		expect(computeCloudRadius(fallbackMany)).toBeCloseTo(30, 6);

		const serverNearOrigin = [makeTag("a", 10, 3, 4, 0)];
		expect(computeCloudRadius(serverNearOrigin)).toBe(60);

		const serverFar = [makeTag("a", 10, 60, 80, 0)]; // dist = 100 -> 100 * 1.2 = 120
		expect(computeCloudRadius(serverFar)).toBeCloseTo(120, 6);

		const model = buildSceneModel(serverFar);
		expect(model.cloudRadius).toBeCloseTo(120, 6);
		expect(model.nodes).toHaveLength(1);
		expect(model.nodes[0]?.position).toEqual([60, 80, 0]);
		expect(model.indexByName.get("a")).toBe(0);
	});

	it("keeps the nearest cloud node visibly inked (fog factor < 0.6) across [minDistance, maxDistance]", () => {
		const cloudRadius = 100;
		const minDistance = cloudRadius * 0.3;
		const maxDistance = cloudRadius * 5;

		for (const d of [
			minDistance,
			cloudRadius * 0.5,
			cloudRadius * 1.5,
			cloudRadius * 3,
			cloudRadius * 4.2,
			maxDistance,
		]) {
			const win = fogWindow(d, cloudRadius);
			expect(win.near).toBeCloseTo(Math.max(0, d - 0.2 * cloudRadius), 6);
			expect(win.far).toBeCloseTo(d + 1.6 * cloudRadius, 6);

			const nearestNodeDepth = Math.max(0, d - cloudRadius);
			expect(computeFogFactor(nearestNodeDepth, win)).toBeLessThan(0.6);
			expect(computeFogFactor(d, win)).toBeLessThan(0.6);
		}
	});

	it("fades label opacity with the fog window, clamps visible nodes to 0.35, and skips vanished nodes", () => {
		const win = fogWindow(150, 100); // near = 130, far = 310 (span = 180)

		expect(computeLabelOpacity(130, win)).toBeCloseTo(1.0, 6);
		// mid-depth: depth = 220 => fogFactor = 0.5 => opacity = 0.5
		expect(computeLabelOpacity(220, win)).toBeCloseTo(0.5, 6);
		// deep but still visible: depth = 274 => fogFactor = 144/180 = 0.8 => clamped to 0.35
		expect(computeLabelOpacity(274, win)).toBeCloseTo(0.35, 6);
		// vanished node (fogFactor ~ 1): skipped (opacity = 0)
		expect(computeLabelOpacity(309, win)).toBe(0);
		expect(computeLabelOpacity(350, win)).toBe(0);
	});
});
