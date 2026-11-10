import type { TagCloudItem } from "#lib/connect/index.js";

export type ColorBucket = 0 | 1 | 2 | 3 | 4;

export interface SceneNode {
	index: number;
	tagName: string;
	articleCount: number;
	position: [number, number, number];
	radius: number;
	colorBucket: ColorBucket;
	labelFontSize: number;
}

export interface SceneModel {
	nodes: SceneNode[];
	cloudRadius: number;
	maxCount: number;
	hasServerPositions: boolean;
	indexByName: Map<string, number>;
}

const MIN_RADIUS = 0.3;
const MAX_RADIUS = 3.0;
const MIN_LABEL_FONT_SIZE = 11;
const MAX_LABEL_FONT_SIZE = 16;

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

export function buildSceneModel(tags: readonly TagCloudItem[]): SceneModel {
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

	for (let i = 0; i < tags.length; i++) {
		const tag = tags[i] as TagCloudItem;
		const position: [number, number, number] = serverPos
			? [tag.positionX, tag.positionY, tag.positionZ]
			: (fallback?.[i] ?? [0, 0, 0]);

		nodes[i] = {
			index: i,
			tagName: tag.tagName,
			articleCount: tag.articleCount,
			position,
			radius: computeRadius(tag.articleCount, maxCount),
			colorBucket: computeColorBucket(tag.articleCount, maxCount),
			labelFontSize: computeLabelFontSize(tag.articleCount, maxCount),
		};
		indexByName.set(tag.tagName, i);
	}

	return {
		nodes,
		cloudRadius,
		maxCount,
		hasServerPositions: serverPos,
		indexByName,
	};
}
