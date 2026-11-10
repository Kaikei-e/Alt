import { Color } from "three";
import { describe, expect, it } from "vitest";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import { buildPalette, contrastRatio, TOPIC_INKS } from "./palette.js";
import {
	applyIncrementalNodeColors,
	applyIncrementalNodeScales,
	BODY_FONT_STACK,
	buildEdgeBuffers,
	buildRegionLabelCandidates,
	buildSceneModel,
	buildTopicClusters,
	buildTopicsKeyRows,
	computeCloudRadius,
	computeColorBucket,
	computeFocusEmphasis,
	computeFogFactor,
	computeLabelFontSize,
	computeLabelOpacity,
	computeLambertLitColor,
	computeNeighbourSet,
	computeRadius,
	DISPLAY_FONT_STACK,
	fibonacciSphere,
	fogWindow,
	hasServerPositions,
	KEY_LIGHT_DIRECTION,
	needsFraming,
	resolveNodeLabelTypography,
	SELECTED_NODE_SCALE,
	updateEdgeBufferAlphas,
} from "./sceneModel.js";

function makeTag(
	tagName: string,
	articleCount: number,
	x = 0,
	y = 0,
	z = 0,
	clusterId = 0,
): TagCloudItem {
	return {
		tagName,
		articleCount,
		positionX: x,
		positionY: y,
		positionZ: z,
		clusterId,
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

	it("fades label opacity with the smoothstep fog window, clamps visible nodes to 0.35, and skips vanished nodes", () => {
		const win = fogWindow(150, 100); // near = 130, far = 310 (span = 180)

		expect(computeFogFactor(130, win)).toBe(0);
		// t = 0.25 => depth = 130 + 0.25 * 180 = 175 => smoothstep = 0.15625
		expect(computeFogFactor(175, win)).toBeCloseTo(0.15625, 6);
		expect(computeLabelOpacity(175, win)).toBeCloseTo(1 - 0.15625, 6);
		// midpoint: t = 0.5 => depth = 220 => smoothstep = 0.5 => opacity = 0.5
		expect(computeFogFactor(220, win)).toBeCloseTo(0.5, 6);
		expect(computeLabelOpacity(220, win)).toBeCloseTo(0.5, 6);
		// deep but still visible: t = 0.8 => depth = 274 => smoothstep = 0.896 => 1 - 0.896 = 0.104 => clamped to 0.35
		expect(computeLabelOpacity(274, win)).toBeCloseTo(0.35, 6);
		// vanished node (fogFactor ~ 1): skipped (opacity = 0)
		expect(computeLabelOpacity(309, win)).toBe(0);
		expect(computeLabelOpacity(350, win)).toBe(0);
	});

	it("computes neighbour set and per-instance focus emphasis for node or cluster selection", () => {
		const tags = [
			makeTag("rust", 50, 0, 0, 0, 1),
			makeTag("tokio", 30, 5, 0, 0, 1),
			makeTag("svelte", 40, -5, 0, 0, 2),
			makeTag("css", 10, 0, 5, 0, 0),
		];
		const edges: TagCloudEdge[] = [
			{ sourceIndex: 0, targetIndex: 1, weight: 0.8 },
			{ sourceIndex: 1, targetIndex: 2, weight: 0.4 },
		];
		const model = buildSceneModel(tags);

		expect([...computeNeighbourSet(model.nodes.length, edges, 1)]).toEqual([
			0, 2,
		]);
		expect([...computeNeighbourSet(model.nodes.length, edges, null)]).toEqual(
			[],
		);

		const idle = computeFocusEmphasis(model.nodes, edges);
		expect(idle.activeFocus).toBe(false);
		expect([...idle.nodePaperMix]).toEqual([0, 0, 0, 0]);
		expect(idle.edgeAlphas[0]).toBeCloseTo(0.1 + 0.35 * 0.8, 5);
		expect(idle.edgeAlphas[1]).toBeCloseTo(0.1 + 0.35 * 0.4, 5);

		const focused = computeFocusEmphasis(model.nodes, edges, {
			focusIndex: 0,
		});
		expect(focused.activeFocus).toBe(true);
		expect([...focused.neighborIndices]).toEqual([1]);
		expect([...focused.nodePaperMix]).toEqual([0, 0, 0.75, 0.75]);
		expect(focused.edgeAlphas[0]).toBeCloseTo(0.1 + 0.35 * 0.8, 5);
		expect(focused.edgeAlphas[1]).toBeCloseTo(0.04, 5);

		const clusterFocused = computeFocusEmphasis(model.nodes, edges, {
			focusClusterId: 1,
		});
		expect(clusterFocused.activeFocus).toBe(true);
		expect([...clusterFocused.nodePaperMix]).toEqual([0, 0, 0.75, 0.75]);
		expect(clusterFocused.edgeAlphas[0]).toBeCloseTo(0.1 + 0.35 * 0.8, 5);
		expect(clusterFocused.edgeAlphas[1]).toBeCloseTo(0.04, 5);
	});

	it("builds single-buffer LineSegments positions and RGBA vertex colours for intra- and cross-cluster edges", () => {
		const palette = buildPalette();
		const tags = [
			makeTag("rust", 50, 1, 2, 3, 1),
			makeTag("tokio", 30, 4, 5, 6, 1),
			makeTag("svelte", 40, 7, 8, 9, 2),
		];
		const edges: TagCloudEdge[] = [
			{ sourceIndex: 0, targetIndex: 1, weight: 1.0 },
			{ sourceIndex: 1, targetIndex: 2, weight: 0.2 },
		];
		const model = buildSceneModel(tags);
		const buffers = buildEdgeBuffers(model.nodes, edges, palette);

		expect(buffers.positions).toHaveLength(12);
		expect([...buffers.positions]).toEqual([
			1, 2, 3, 4, 5, 6, 4, 5, 6, 7, 8, 9,
		]);
		expect(buffers.colors).toHaveLength(16);

		// Edge 0 is intra-cluster 1 (indigo #2b3a67 in linear working space) with alpha 0.45
		const expectedIndigo = new Color("#2b3a67");
		expect(buffers.colors[0]).toBeCloseTo(expectedIndigo.r, 5);
		expect(buffers.colors[1]).toBeCloseTo(expectedIndigo.g, 5);
		expect(buffers.colors[2]).toBeCloseTo(expectedIndigo.b, 5);
		expect(buffers.colors[3]).toBeCloseTo(0.45, 5);
		expect(buffers.colors[7]).toBeCloseTo(0.45, 5);

		// Edge 1 is cross-cluster -> linear neutral ink (R == G == B) with alpha 0.10 + 0.35 * 0.2 = 0.17
		const expectedNeutral = new Color(palette.labelInk);
		expect(buffers.colors[8]).toBeCloseTo(expectedNeutral.r, 5);
		expect(buffers.colors[9]).toBeCloseTo(expectedNeutral.g, 5);
		expect(buffers.colors[10]).toBeCloseTo(expectedNeutral.b, 5);
		expect(buffers.colors[11]).toBeCloseTo(0.17, 5);
	});

	it("assigns display tier [18..26]px to top 12 tags and body tier [11..14]px to remaining tags, and marks region labels only for clusters with >= 6 tags", () => {
		const tags: TagCloudItem[] = [];
		for (let i = 0; i < 6; i++) {
			tags.push(makeTag(`c1_${i}`, 100 - i, i * 2, 0, 0, 1));
		}
		for (let i = 0; i < 5; i++) {
			tags.push(makeTag(`c2_${i}`, 80 - i, 0, i * 2, 0, 2));
		}
		for (let i = 0; i < 4; i++) {
			tags.push(makeTag(`c0_${i}`, 20 - i, 0, 0, i * 2, 0));
		}

		const model = buildSceneModel(tags);
		const displayNodes = model.nodes.filter((n) => n.isDisplayTier);
		const bodyNodes = model.nodes.filter((n) => !n.isDisplayTier);

		expect(displayNodes).toHaveLength(12);
		expect(bodyNodes).toHaveLength(3);

		for (const n of displayNodes) {
			expect(n.labelFontSize).toBeGreaterThanOrEqual(18);
			expect(n.labelFontSize).toBeLessThanOrEqual(26);
		}
		for (const n of bodyNodes) {
			expect(n.labelFontSize).toBeGreaterThanOrEqual(11);
			expect(n.labelFontSize).toBeLessThanOrEqual(14);
		}

		const clusters = buildTopicClusters(model.nodes, buildPalette());
		expect(clusters).toHaveLength(2);
		expect(clusters[0]).toMatchObject({
			clusterId: 1,
			tagCount: 6,
			topTags: ["c1_0", "c1_1"],
			label: "c1_0 · c1_1",
			regionText: "C1_0 · C1_1",
			ink: TOPIC_INKS[0],
			centroid: [5, 0, 0],
			showRegionLabel: true,
		});
		expect(clusters[1]).toMatchObject({
			clusterId: 2,
			tagCount: 5,
			topTags: ["c2_0", "c2_1"],
			label: "c2_0 · c2_1",
			regionText: "C2_0 · C2_1",
			ink: TOPIC_INKS[1],
			centroid: [0, 4, 0],
			showRegionLabel: false,
		});
	});

	it("decides camera framing only for index selections that are behind the camera or outside the central 60% viewport", () => {
		expect(
			needsFraming({
				source: "canvas",
				ndcX: 0.95,
				ndcY: 0.95,
				isBehindCamera: true,
			}),
		).toBe(false);
		expect(
			needsFraming({
				source: "index",
				ndcX: 0.2,
				ndcY: -0.5,
				isBehindCamera: false,
			}),
		).toBe(false);
		expect(
			needsFraming({
				source: "index",
				ndcX: 0.6,
				ndcY: -0.6,
				isBehindCamera: false,
			}),
		).toBe(false);
		expect(
			needsFraming({
				source: "index",
				ndcX: 0.65,
				ndcY: 0,
				isBehindCamera: false,
			}),
		).toBe(true);
		expect(
			needsFraming({
				source: "index",
				ndcX: 0,
				ndcY: -0.75,
				isBehindCamera: false,
			}),
		).toBe(true);
		expect(
			needsFraming({
				source: "index",
				ndcX: 0,
				ndcY: 0,
				isBehindCamera: true,
			}),
		).toBe(true);
		// Node at ndcX = 0.25 on a 1000px canvas (screenX = 625px) is occluded by the 400px right detail panel (visible width = 600px)
		expect(
			needsFraming({
				source: "index",
				ndcX: 0.25,
				ndcY: 0,
				isBehindCamera: false,
				canvasWidth: 1000,
				occludedRightPx: 400,
			}),
		).toBe(true);
		// Node at ndcX = -0.4 on a 1000px canvas (screenX = 300px, midpoint of 600px visible width) is comfortably framed
		expect(
			needsFraming({
				source: "index",
				ndcX: -0.4,
				ndcY: 0,
				isBehindCamera: false,
				canvasWidth: 1000,
				occludedRightPx: 400,
			}),
		).toBe(false);
	});

	it("reproduces topic ink on the key-lit face under PI-scaled Lambert lights and keeps mid-tone >= 3:1 on paper", () => {
		for (const ink of TOPIC_INKS) {
			const expected = new Color(ink);
			const keyLit = computeLambertLitColor(ink, KEY_LIGHT_DIRECTION);
			expect(Math.abs(keyLit.linear[0] - expected.r)).toBeLessThan(0.01);
			expect(Math.abs(keyLit.linear[1] - expected.g)).toBeLessThan(0.01);
			expect(Math.abs(keyLit.linear[2] - expected.b)).toBeLessThan(0.01);

			const midTone = computeLambertLitColor(ink, [1, 0, 0]);
			expect(contrastRatio(midTone.hex, "#faf9f7")).toBeGreaterThanOrEqual(3.0);
		}
	});

	it("precomputes base inks, uses #1a1a1a and 1.2x scale for selected node, and touches only changed indices on focus changes", () => {
		const palette = buildPalette();
		const tags = [
			makeTag("rust", 50, 0, 0, 0, 1),
			makeTag("tokio", 30, 5, 0, 0, 1),
			makeTag("svelte", 40, -5, 0, 0, 2),
			makeTag("css", 10, 0, 5, 0, 3),
		];
		const edges: TagCloudEdge[] = [
			{ sourceIndex: 0, targetIndex: 1, weight: 0.8 },
			{ sourceIndex: 2, targetIndex: 3, weight: 0.5 },
		];
		const model = buildSceneModel(tags, palette);

		for (const node of model.nodes) {
			expect(node.baseInk).toMatch(/^#[0-9a-f]{6}$/i);
		}

		const currentHexes = new Array<string>(model.nodes.length).fill("");
		const currentScales = new Float32Array(model.nodes.length);

		// Initial populate touches all 4 nodes
		const idleEmphasis = computeFocusEmphasis(model.nodes, edges);
		expect(
			applyIncrementalNodeColors(
				currentHexes,
				model.nodes,
				palette,
				null,
				idleEmphasis.nodePaperMix,
			),
		).toEqual([0, 1, 2, 3]);
		expect(
			applyIncrementalNodeScales(currentScales, model.nodes, null, null),
		).toEqual([0, 1, 2, 3]);

		// Focus cluster 1: nodes 0 and 1 stay at paperMix = 0 (unmodified!), only nodes 2 and 3 fade to paperMix = 0.75
		const cluster1Emphasis = computeFocusEmphasis(model.nodes, edges, {
			focusClusterId: 1,
		});
		expect(
			applyIncrementalNodeColors(
				currentHexes,
				model.nodes,
				palette,
				null,
				cluster1Emphasis.nodePaperMix,
			),
		).toEqual([2, 3]);
		// Scales did not change on cluster focus
		expect(
			applyIncrementalNodeScales(currentScales, model.nodes, null, null),
		).toEqual([]);

		// Select node 0 while cluster 1 is already focused: nodes 1, 2, 3 keep their exact paperMix; ONLY node 0 changes colour to labelInk (#1a1a1a) and scale to 1.2x
		const select0Emphasis = computeFocusEmphasis(model.nodes, edges, {
			focusIndex: 0,
		});
		expect(
			applyIncrementalNodeColors(
				currentHexes,
				model.nodes,
				palette,
				0,
				select0Emphasis.nodePaperMix,
			),
		).toEqual([0]);
		expect(currentHexes[0]).toBe(palette.labelInk);
		expect(
			applyIncrementalNodeScales(currentScales, model.nodes, 0, null),
		).toEqual([0]);
		expect(currentScales[0]).toBeCloseTo(
			(model.nodes[0]?.radius ?? 0) * SELECTED_NODE_SCALE,
			5,
		);

		// Edge buffer alphas: switching from idle to focusing node 0 keeps edge 0 incident (unmodified) and fades only edge 1
		const buffers = buildEdgeBuffers(
			model.nodes,
			edges,
			palette,
			idleEmphasis.edgeAlphas,
		);
		expect(
			updateEdgeBufferAlphas(buffers.colors, select0Emphasis.edgeAlphas),
		).toEqual([1]);
		expect(buffers.colors[11]).toBeCloseTo(0.04, 5);
		expect(buffers.colors[15]).toBeCloseTo(0.04, 5);
	});

	it("resolves Playfair Display [18..26]px for top-12 tags, Source Sans 3 [11..14]px for others, region label candidates only for clusters >= 6 when unselected, and topics key rows", () => {
		const tags: TagCloudItem[] = [];
		for (let i = 0; i < 6; i++) {
			tags.push(makeTag(`ai_${i}`, 120 - i * 5, i * 2, 0, 0, 1));
		}
		for (let i = 0; i < 6; i++) {
			tags.push(makeTag(`web_${i}`, 80 - i * 5, 0, i * 2, 0, 2));
		}
		for (let i = 0; i < 3; i++) {
			tags.push(makeTag(`misc_${i}`, 20 - i, 0, 0, i * 2, 3));
		}

		const palette = buildPalette();
		const model = buildSceneModel(tags);

		for (let i = 0; i < model.nodes.length; i++) {
			const node = model.nodes[i];
			if (!node) continue;
			const typo = resolveNodeLabelTypography(node, i === 0);
			if (i < 12) {
				expect(typo.isDisplayTier).toBe(true);
				expect(typo.fontFamily).toBe(DISPLAY_FONT_STACK);
				expect(typo.fontSize).toBeGreaterThanOrEqual(18);
				expect(typo.fontSize).toBeLessThanOrEqual(26);
				expect(typo.fontWeight).toBe(i === 0 ? 700 : 600);
			} else {
				expect(typo.isDisplayTier).toBe(false);
				expect(typo.fontFamily).toBe(BODY_FONT_STACK);
				expect(typo.fontSize).toBeGreaterThanOrEqual(11);
				expect(typo.fontSize).toBeLessThanOrEqual(14);
				expect(typo.fontWeight).toBe(600);
			}
		}

		const clusters = buildTopicClusters(model.nodes, palette);
		const regionCandidates = buildRegionLabelCandidates(clusters, null);
		expect(regionCandidates).toHaveLength(2);
		expect(regionCandidates[0]).toMatchObject({
			clusterId: 1,
			text: "AI_0 · AI_1",
			fontFamily: BODY_FONT_STACK,
			fontSize: 10,
			fontWeight: 700,
			letterSpacingEm: 0.12,
			alpha: 0.55,
			ink: TOPIC_INKS[0],
			centroid: [5, 0, 0],
		});
		expect(regionCandidates[1]).toMatchObject({
			clusterId: 2,
			text: "WEB_0 · WEB_1",
			fontFamily: BODY_FONT_STACK,
			fontSize: 10,
			fontWeight: 700,
			letterSpacingEm: 0.12,
			alpha: 0.55,
			ink: TOPIC_INKS[1],
			centroid: [0, 5, 0],
		});
		expect(buildRegionLabelCandidates(clusters, 0)).toEqual([]);

		const keyRows = buildTopicsKeyRows(clusters);
		expect(keyRows).toEqual([
			{
				clusterId: 1,
				ink: TOPIC_INKS[0],
				topTags: ["ai_0", "ai_1"],
				label: "ai_0 · ai_1",
				ariaLabel: "Highlight topic: ai_0 · ai_1",
			},
			{
				clusterId: 2,
				ink: TOPIC_INKS[1],
				topTags: ["web_0", "web_1"],
				label: "web_0 · web_1",
				ariaLabel: "Highlight topic: web_0 · web_1",
			},
			{
				clusterId: 3,
				ink: TOPIC_INKS[2],
				topTags: ["misc_0", "misc_1"],
				label: "misc_0 · misc_1",
				ariaLabel: "Highlight topic: misc_0 · misc_1",
			},
		]);

		const unclusteredNodes = buildSceneModel([
			makeTag("solo", 10, 0, 0, 0, 0),
		]).nodes;
		expect(
			buildTopicsKeyRows(buildTopicClusters(unclusteredNodes, palette)),
		).toEqual([]);
	});
});
