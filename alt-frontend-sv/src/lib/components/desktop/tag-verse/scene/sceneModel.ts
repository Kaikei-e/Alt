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

export function computeColorBucket(
	_articleCount: number,
	_maxCount: number,
): ColorBucket {
	return 0;
}

export function computeRadius(_articleCount: number, _maxCount: number): number {
	return 0;
}

export function computeLabelFontSize(
	_articleCount: number,
	_maxCount: number,
): number {
	return 0;
}

export function hasServerPositions(_tags: readonly TagCloudItem[]): boolean {
	return false;
}

export function fibonacciSphere(
	_count: number,
	_radius: number,
): [number, number, number][] {
	return [];
}

export function computeCloudRadius(
	_tags: readonly TagCloudItem[],
	_serverPositions?: boolean,
): number {
	return 0;
}

export function buildSceneModel(_tags: readonly TagCloudItem[]): SceneModel {
	return {
		nodes: [],
		cloudRadius: 0,
		maxCount: 1,
		hasServerPositions: false,
		indexByName: new Map(),
	};
}
