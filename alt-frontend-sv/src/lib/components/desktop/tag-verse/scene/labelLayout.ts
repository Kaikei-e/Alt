export interface LabelBox {
	x: number;
	y: number;
	w: number;
	h: number;
	forced?: boolean;
}

export interface LayoutLabelsOptions {
	cellSize?: number;
	forcedIndices?: ReadonlySet<number> | readonly number[];
}

export function boxesOverlap(_a: LabelBox, _b: LabelBox): boolean {
	return false;
}

export function layoutLabels(
	_boxes: readonly LabelBox[],
	_options?: LayoutLabelsOptions | ReadonlySet<number> | readonly number[],
): number[] {
	return [];
}
