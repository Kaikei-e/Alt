import { Color, LinearSRGBColorSpace } from "three";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import {
	buildPalette,
	clusterBaseInk,
	clusterInkForTag,
	mixTowardPaper,
	type TagVersePalette,
} from "./palette.js";

export type ColorBucket = 0 | 1 | 2 | 3 | 4;
export type SelectionSource = "canvas" | "index";

export interface SceneNode {
	tagName: string;
	articleCount: number;
	position: [number, number, number];
	radius: number;
	colorBucket: ColorBucket;
	labelFontSize: number;
	clusterId: number;
	isDisplayTier: boolean;
	baseInk: string;
}

export interface TopicClusterSummary {
	clusterId: number;
	tagCount: number;
	topTags: string[];
	label: string;
	regionText: string;
	ink: string;
	centroid: [number, number, number];
	showRegionLabel: boolean;
}

export interface FocusEmphasis {
	activeFocus: boolean;
	fullStrengthNodes: Set<number>;
	neighborIndices: Set<number>;
	nodePaperMix: Float32Array;
	edgeAlphas: Float32Array;
}

export interface EdgeBuffers {
	positions: Float32Array;
	colors: Float32Array;
}

export interface SceneModel {
	nodes: SceneNode[];
	cloudRadius: number;
	indexByName: Map<string, number>;
}

export interface FogWindow {
	near: number;
	far: number;
}

export const DISPLAY_FONT_STACK = '"Playfair Display", Georgia, serif';
export const BODY_FONT_STACK = '"Source Sans 3", system-ui, sans-serif';
export const REGION_LABEL_FONT_SIZE = 10;
export const REGION_LABEL_LETTER_SPACING_EM = 0.12;
export const REGION_LABEL_ALPHA = 0.55;
// WHY: Three.js MeshLambertMaterial multiplies irradiance by RECIPROCAL_PI (1/π) in BRDF_Lambert, so scaling the 0.7 hemisphere fill and 0.3 directional key intensities by Math.PI cancels the 1/π factor and reproduces the topic ink on a key-lit face.
export const HEMI_LIGHT_INTENSITY = 0.7 * Math.PI;
export const DIR_LIGHT_INTENSITY = 0.3 * Math.PI;
export const KEY_LIGHT_DIRECTION: readonly [number, number, number] = [
	1, 1.5, 1,
];
export const SELECTED_NODE_SCALE = 1.2;
export const HOVERED_NODE_SCALE = 1.15;

export interface FramingDestinations {
	target: [number, number, number];
	cameraPosition: [number, number, number];
	rightShiftWorld: number;
}

export interface NodeLabelTypography {
	fontFamily: string;
	fontSize: number;
	fontWeight: 600 | 700;
	isDisplayTier: boolean;
}

export interface RegionLabelCandidate {
	clusterId: number;
	text: string;
	fontFamily: string;
	fontSize: number;
	fontWeight: 700;
	letterSpacingEm: number;
	alpha: number;
	ink: string;
	centroid: [number, number, number];
}

export interface TopicsKeyRow {
	clusterId: number;
	ink: string;
	topTags: string[];
	label: string;
	ariaLabel: string;
}

const MIN_RADIUS = 0.3;
const MAX_RADIUS = 3.0;
const MIN_LABEL_FONT_SIZE = 11;
const MAX_LABEL_FONT_SIZE = 16;
const MIN_LABEL_OPACITY = 0.35;
const FOG_VANISH_THRESHOLD = 0.98;

export function computeColorBucket(
	articleCount: number,
	maxCount: number,
): ColorBucket {
	const ratio = Math.max(0, articleCount) / Math.max(maxCount, 1);
	const t = ratio ** 0.5;
	if (t < 0.2) return 0;
	if (t < 0.4) return 1;
	if (t < 0.6) return 2;
	if (t < 0.8) return 3;
	return 4;
}

export function computeRadius(articleCount: number, maxCount: number): number {
	const normalized =
		Math.min(1, Math.max(0, articleCount) / Math.max(maxCount, 1)) ** 0.4;
	return MIN_RADIUS + normalized * (MAX_RADIUS - MIN_RADIUS);
}

export function computeLabelFontSize(
	articleCount: number,
	maxCount: number,
	_isDisplayTier = false,
): number {
	const normalized =
		Math.min(1, Math.max(0, articleCount) / Math.max(maxCount, 1)) ** 0.4;
	return (
		MIN_LABEL_FONT_SIZE +
		normalized * (MAX_LABEL_FONT_SIZE - MIN_LABEL_FONT_SIZE)
	);
}

export function hasServerPositions(tags: readonly TagCloudItem[]): boolean {
	return tags.some(
		(t) => t.positionX !== 0 || t.positionY !== 0 || t.positionZ !== 0,
	);
}

export function fibonacciSphere(
	count: number,
	radius: number,
): [number, number, number][] {
	const points: [number, number, number][] = [];
	const goldenAngle = Math.PI * (3 - Math.sqrt(5));
	for (let i = 0; i < count; i++) {
		const y = 1 - (i / Math.max(count - 1, 1)) * 2;
		const radiusAtY = Math.sqrt(Math.max(0, 1 - y * y));
		const theta = goldenAngle * i;
		const x = Math.cos(theta) * radiusAtY;
		const z = Math.sin(theta) * radiusAtY;
		points.push([x * radius, y * radius, z * radius]);
	}
	return points;
}

export function computeCloudRadius(
	tags: readonly TagCloudItem[],
	serverPositions?: boolean,
): number {
	const useServer = serverPositions ?? hasServerPositions(tags);
	if (useServer) {
		let maxDist = 0;
		for (const t of tags) {
			const dist = Math.hypot(t.positionX, t.positionY, t.positionZ);
			if (dist > maxDist) {
				maxDist = dist;
			}
		}
		return Math.max(60, maxDist * 1.2);
	}
	return Math.max(15, Math.sqrt(tags.length) * 3);
}

export function fogWindow(distance: number, cloudRadius: number): FogWindow {
	const r = Math.max(0, cloudRadius);
	const d = Math.max(0, distance);
	const near = Math.max(0, d - 0.2 * r);
	const far = Math.max(near + 1e-3, d + 1.6 * r);
	return { near, far };
}

export function computeFogFactor(depth: number, window: FogWindow): number {
	const span = Math.max(1e-6, window.far - window.near);
	const t = Math.max(0, Math.min(1, (depth - window.near) / span));
	return t * t * (3 - 2 * t);
}

export function computeLabelOpacity(depth: number, window: FogWindow): number {
	const fogFactor = computeFogFactor(depth, window);
	if (fogFactor >= FOG_VANISH_THRESHOLD) {
		return 0;
	}
	return Math.max(MIN_LABEL_OPACITY, 1 - fogFactor);
}

export function buildSceneModel(
	tags: readonly TagCloudItem[],
	palette: TagVersePalette = buildPalette(),
): SceneModel {
	const serverPos = hasServerPositions(tags);
	const cloudRadius = computeCloudRadius(tags, serverPos);
	let maxCount = 1;
	for (const t of tags) {
		if (t.articleCount > maxCount) {
			maxCount = t.articleCount;
		}
	}

	const fallback = serverPos ? null : fibonacciSphere(tags.length, cloudRadius);
	const nodes: SceneNode[] = new Array(tags.length);
	const indexByName = new Map<string, number>();

	const rankedIndices = Array.from({ length: tags.length }, (_, i) => i).sort(
		(a, b) =>
			(tags[b]?.articleCount ?? 0) - (tags[a]?.articleCount ?? 0) || a - b,
	);
	const displaySet = new Set(rankedIndices.slice(0, 12));

	for (let i = 0; i < tags.length; i++) {
		const tag = tags[i] as TagCloudItem;
		const position: [number, number, number] = serverPos
			? [tag.positionX, tag.positionY, tag.positionZ]
			: (fallback?.[i] ?? [0, 0, 0]);
		const isDisplayTier = displaySet.has(i);
		const normalized =
			Math.min(1, Math.max(0, tag.articleCount) / Math.max(maxCount, 1)) ** 0.4;
		const labelFontSize = isDisplayTier
			? 18 + normalized * 8
			: 11 + normalized * 3;
		const colorBucket = computeColorBucket(tag.articleCount, maxCount);

		nodes[i] = {
			tagName: tag.tagName,
			articleCount: tag.articleCount,
			position,
			radius: computeRadius(tag.articleCount, maxCount),
			colorBucket,
			labelFontSize,
			clusterId: tag.clusterId,
			isDisplayTier,
			baseInk: clusterInkForTag(tag.clusterId, colorBucket, palette),
		};
		indexByName.set(tag.tagName, i);
	}

	return {
		nodes,
		cloudRadius,
		indexByName,
	};
}

export function computeNeighbourSet(
	nodeCount: number,
	edges: readonly TagCloudEdge[],
	focusIndex: number | null,
): Set<number> {
	const neighbours = new Set<number>();
	if (focusIndex === null || focusIndex < 0 || focusIndex >= nodeCount) {
		return neighbours;
	}
	for (const edge of edges) {
		if (
			edge.sourceIndex < 0 ||
			edge.targetIndex < 0 ||
			edge.sourceIndex >= nodeCount ||
			edge.targetIndex >= nodeCount ||
			edge.sourceIndex === edge.targetIndex
		) {
			continue;
		}
		if (edge.sourceIndex === focusIndex) {
			neighbours.add(edge.targetIndex);
		} else if (edge.targetIndex === focusIndex) {
			neighbours.add(edge.sourceIndex);
		}
	}
	return neighbours;
}

export function computeFocusEmphasis(
	nodes: readonly SceneNode[],
	edges: readonly TagCloudEdge[],
	options: {
		focusIndex?: number | null;
		focusClusterId?: number | null;
	} = {},
): FocusEmphasis {
	const nodeCount = nodes.length;
	const edgeCount = edges.length;
	const nodePaperMix = new Float32Array(nodeCount);
	const edgeAlphas = new Float32Array(edgeCount);
	const fullStrengthNodes = new Set<number>();
	let neighborIndices = new Set<number>();

	const focusIndex =
		options.focusIndex !== undefined &&
		options.focusIndex !== null &&
		options.focusIndex >= 0 &&
		options.focusIndex < nodeCount
			? options.focusIndex
			: null;
	const focusClusterId =
		options.focusClusterId !== undefined &&
		options.focusClusterId !== null &&
		options.focusClusterId > 0
			? options.focusClusterId
			: null;

	const activeFocus = focusIndex !== null || focusClusterId !== null;

	if (!activeFocus) {
		for (let i = 0; i < edgeCount; i++) {
			const w = Math.max(0, Math.min(1, edges[i]?.weight ?? 0));
			edgeAlphas[i] = 0.1 + 0.35 * w;
		}
		return {
			activeFocus: false,
			fullStrengthNodes,
			neighborIndices,
			nodePaperMix,
			edgeAlphas,
		};
	}

	if (focusIndex !== null) {
		neighborIndices = computeNeighbourSet(nodeCount, edges, focusIndex);
		fullStrengthNodes.add(focusIndex);
		for (const n of neighborIndices) {
			fullStrengthNodes.add(n);
		}
		for (let i = 0; i < nodeCount; i++) {
			nodePaperMix[i] = fullStrengthNodes.has(i) ? 0 : 0.75;
		}
		for (let i = 0; i < edgeCount; i++) {
			const edge = edges[i];
			if (!edge) continue;
			const isIncident =
				edge.sourceIndex === focusIndex || edge.targetIndex === focusIndex;
			if (isIncident) {
				const w = Math.max(0, Math.min(1, edge.weight));
				edgeAlphas[i] = 0.1 + 0.35 * w;
			} else {
				edgeAlphas[i] = 0.04;
			}
		}
	} else if (focusClusterId !== null) {
		for (let i = 0; i < nodeCount; i++) {
			if (nodes[i]?.clusterId === focusClusterId) {
				fullStrengthNodes.add(i);
				nodePaperMix[i] = 0;
			} else {
				nodePaperMix[i] = 0.75;
			}
		}
		for (let i = 0; i < edgeCount; i++) {
			const edge = edges[i];
			if (!edge) continue;
			const isInternal =
				fullStrengthNodes.has(edge.sourceIndex) &&
				fullStrengthNodes.has(edge.targetIndex);
			if (isInternal) {
				const w = Math.max(0, Math.min(1, edge.weight));
				edgeAlphas[i] = 0.1 + 0.35 * w;
			} else {
				edgeAlphas[i] = 0.04;
			}
		}
	}

	return {
		activeFocus: true,
		fullStrengthNodes,
		neighborIndices,
		nodePaperMix,
		edgeAlphas,
	};
}

const _edgeColor = new Color();

export function buildEdgeBuffers(
	nodes: readonly SceneNode[],
	edges: readonly TagCloudEdge[],
	palette: TagVersePalette,
	edgeAlphas?: Float32Array,
): EdgeBuffers {
	const validCount = edges.length;
	const positions = new Float32Array(validCount * 6);
	const colors = new Float32Array(validCount * 8);
	_edgeColor.set(palette.labelInk);
	const neutralR = _edgeColor.r;
	const neutralG = _edgeColor.g;
	const neutralB = _edgeColor.b;

	for (let i = 0; i < validCount; i++) {
		const edge = edges[i];
		if (!edge) continue;
		const source = nodes[edge.sourceIndex];
		const target = nodes[edge.targetIndex];
		if (!source || !target) continue;

		const pOffset = i * 6;
		positions[pOffset] = source.position[0];
		positions[pOffset + 1] = source.position[1];
		positions[pOffset + 2] = source.position[2];
		positions[pOffset + 3] = target.position[0];
		positions[pOffset + 4] = target.position[1];
		positions[pOffset + 5] = target.position[2];

		const isSameCluster =
			source.clusterId > 0 && source.clusterId === target.clusterId;
		let r = neutralR;
		let g = neutralG;
		let b = neutralB;
		if (isSameCluster) {
			_edgeColor.set(clusterBaseInk(source.clusterId, palette));
			r = _edgeColor.r;
			g = _edgeColor.g;
			b = _edgeColor.b;
		}
		const w = Math.max(0, Math.min(1, edge.weight));
		const alpha =
			edgeAlphas && i < edgeAlphas.length
				? (edgeAlphas[i] ?? 0)
				: 0.1 + 0.35 * w;

		const cOffset = i * 8;
		colors[cOffset] = r;
		colors[cOffset + 1] = g;
		colors[cOffset + 2] = b;
		colors[cOffset + 3] = alpha;
		colors[cOffset + 4] = r;
		colors[cOffset + 5] = g;
		colors[cOffset + 6] = b;
		colors[cOffset + 7] = alpha;
	}

	return { positions, colors };
}

export function buildTopicClusters(
	nodes: readonly (SceneNode | TagCloudItem)[],
	palette: TagVersePalette = buildPalette(),
): TopicClusterSummary[] {
	const byCluster = new Map<number, (SceneNode | TagCloudItem)[]>();
	for (const node of nodes) {
		if (node.clusterId > 0) {
			const group = byCluster.get(node.clusterId);
			if (group) {
				group.push(node);
			} else {
				byCluster.set(node.clusterId, [node]);
			}
		}
	}

	const clusterIds = [...byCluster.keys()].sort((a, b) => a - b);
	const summaries: TopicClusterSummary[] = [];

	for (const clusterId of clusterIds) {
		const members = byCluster.get(clusterId) ?? [];
		if (members.length === 0) continue;

		const sorted = [...members].sort(
			(a, b) =>
				b.articleCount - a.articleCount || a.tagName.localeCompare(b.tagName),
		);
		const topTags = sorted.slice(0, 2).map((n) => n.tagName);
		const label = topTags.join(" · ");
		const regionText = label.toUpperCase();

		let sumX = 0;
		let sumY = 0;
		let sumZ = 0;
		for (const m of members) {
			if ("position" in m) {
				sumX += m.position[0];
				sumY += m.position[1];
				sumZ += m.position[2];
			} else {
				sumX += m.positionX;
				sumY += m.positionY;
				sumZ += m.positionZ;
			}
		}
		const n = members.length;

		summaries.push({
			clusterId,
			tagCount: n,
			topTags,
			label,
			regionText,
			ink: clusterBaseInk(clusterId, palette),
			centroid: [sumX / n, sumY / n, sumZ / n],
			showRegionLabel: n >= 6,
		});
	}

	return summaries;
}

export function needsFraming(params: {
	source: SelectionSource;
	ndcX: number;
	ndcY: number;
	isBehindCamera: boolean;
	canvasWidth?: number;
	occludedRightPx?: number;
}): boolean {
	if (params.source !== "index") {
		return false;
	}
	if (params.isBehindCamera) {
		return true;
	}
	let effectiveNdcX = params.ndcX;
	const canvasWidth = params.canvasWidth ?? 0;
	const occludedRightPx = Math.max(0, params.occludedRightPx ?? 0);
	if (canvasWidth > 0 && occludedRightPx > 0) {
		const visibleWidth = Math.max(1, canvasWidth - occludedRightPx);
		const screenX = (params.ndcX * 0.5 + 0.5) * canvasWidth;
		effectiveNdcX = (screenX / visibleWidth) * 2 - 1;
	}
	return Math.abs(effectiveNdcX) > 0.6 || Math.abs(params.ndcY) > 0.6;
}

export function computeFramingDestinations(_params: {
	nodePosition: readonly [number, number, number];
	viewOffset: readonly [number, number, number];
	cameraRight: readonly [number, number, number];
	fovDeg: number;
	canvasHeight: number;
	occludedRightPx?: number;
}): FramingDestinations {
	return {
		target: [0, 0, 0],
		cameraPosition: [0, 0, 0],
		rightShiftWorld: 0,
	};
}

export function applyIncrementalNodeColors(
	currentHexes: string[],
	nodes: readonly SceneNode[],
	palette: TagVersePalette,
	selectedIndex: number | null,
	nodePaperMix?: Float32Array,
): number[] {
	const changed: number[] = [];
	const count = nodes.length;
	if (currentHexes.length !== count) {
		currentHexes.length = count;
	}
	for (let i = 0; i < count; i++) {
		const node = nodes[i];
		if (!node) continue;
		const isSelected = i === selectedIndex;
		const paperMix =
			nodePaperMix && i < nodePaperMix.length ? (nodePaperMix[i] ?? 0) : 0;
		let nextHex: string;
		if (isSelected) {
			nextHex = palette.labelInk;
		} else if (paperMix > 0) {
			nextHex = mixTowardPaper(node.baseInk, palette.paper, paperMix);
		} else {
			nextHex = node.baseInk;
		}
		if (currentHexes[i] !== nextHex) {
			currentHexes[i] = nextHex;
			changed.push(i);
		}
	}
	return changed;
}

export function applyIncrementalNodeScales(
	currentScales: Float32Array,
	nodes: readonly SceneNode[],
	selectedIndex: number | null,
	hoveredIndex: number | null,
): number[] {
	const changed: number[] = [];
	const count = nodes.length;
	for (let i = 0; i < count; i++) {
		const node = nodes[i];
		if (!node) continue;
		const factor =
			i === selectedIndex
				? SELECTED_NODE_SCALE
				: i === hoveredIndex
					? HOVERED_NODE_SCALE
					: 1;
		const nextScale = node.radius * factor;
		if (Math.abs((currentScales[i] ?? 0) - nextScale) > 1e-6) {
			currentScales[i] = nextScale;
			changed.push(i);
		}
	}
	return changed;
}

export function updateEdgeBufferAlphas(
	colors: Float32Array,
	edgeAlphas: Float32Array,
): number[] {
	const changed: number[] = [];
	const edgeCount = Math.min(
		edgeAlphas.length,
		Math.floor(colors.length / 8),
	);
	for (let i = 0; i < edgeCount; i++) {
		const nextAlpha = edgeAlphas[i] ?? 0;
		const a0Idx = i * 8 + 3;
		const a1Idx = i * 8 + 7;
		if (
			Math.abs((colors[a0Idx] ?? 0) - nextAlpha) > 1e-6 ||
			Math.abs((colors[a1Idx] ?? 0) - nextAlpha) > 1e-6
		) {
			colors[a0Idx] = nextAlpha;
			colors[a1Idx] = nextAlpha;
			changed.push(i);
		}
	}
	return changed;
}

export function resolveNodeLabelTypography(
	node: SceneNode,
	isSelected = false,
): NodeLabelTypography {
	return {
		fontFamily: node.isDisplayTier ? DISPLAY_FONT_STACK : BODY_FONT_STACK,
		fontSize: node.labelFontSize,
		fontWeight: isSelected ? 700 : 600,
		isDisplayTier: node.isDisplayTier,
	};
}

export function buildRegionLabelCandidates(
	clusters: readonly TopicClusterSummary[],
	selectedIndex: number | null = null,
): RegionLabelCandidate[] {
	if (selectedIndex !== null) {
		return [];
	}
	const candidates: RegionLabelCandidate[] = [];
	for (const cluster of clusters) {
		if (!cluster.showRegionLabel || !cluster.regionText) {
			continue;
		}
		candidates.push({
			clusterId: cluster.clusterId,
			text: cluster.regionText,
			fontFamily: BODY_FONT_STACK,
			fontSize: REGION_LABEL_FONT_SIZE,
			fontWeight: 700,
			letterSpacingEm: REGION_LABEL_LETTER_SPACING_EM,
			alpha: REGION_LABEL_ALPHA,
			ink: cluster.ink,
			centroid: cluster.centroid,
		});
	}
	return candidates;
}

export function buildTopicsKeyRows(
	clusters: readonly TopicClusterSummary[],
): TopicsKeyRow[] {
	return clusters
		.filter((cluster) => cluster.clusterId > 0 && cluster.topTags.length > 0)
		.map((cluster) => ({
			clusterId: cluster.clusterId,
			ink: cluster.ink,
			topTags: cluster.topTags,
			label: cluster.label,
			ariaLabel: `Highlight topic: ${cluster.label}`,
		}));
}
