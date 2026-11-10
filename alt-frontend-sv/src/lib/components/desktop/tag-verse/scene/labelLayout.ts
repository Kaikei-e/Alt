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

const DEFAULT_CELL_SIZE = 64;

export function boxesOverlap(a: LabelBox, b: LabelBox): boolean {
	return (
		a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y
	);
}

function cellKey(cx: number, cy: number): number {
	return ((cx + 32768) << 16) ^ ((cy + 32768) & 0xffff);
}

function resolveOptions(
	options?: LayoutLabelsOptions | ReadonlySet<number> | readonly number[],
): { cellSize: number; forcedSet: Set<number> } {
	const forcedSet = new Set<number>();
	let cellSize = DEFAULT_CELL_SIZE;

	if (!options) {
		return { cellSize, forcedSet };
	}

	if (options instanceof Set || Array.isArray(options)) {
		for (const idx of options) {
			forcedSet.add(idx);
		}
		return { cellSize, forcedSet };
	}

	const opts = options as LayoutLabelsOptions;
	if (opts.cellSize && opts.cellSize > 0) {
		cellSize = opts.cellSize;
	}
	if (opts.forcedIndices) {
		for (const idx of opts.forcedIndices) {
			forcedSet.add(idx);
		}
	}
	return { cellSize, forcedSet };
}

export function layoutLabels(
	boxes: readonly LabelBox[],
	options?: LayoutLabelsOptions | ReadonlySet<number> | readonly number[],
): number[] {
	if (boxes.length === 0) return [];

	const { cellSize, forcedSet } = resolveOptions(options);
	const invCell = 1 / cellSize;
	const grid = new Map<number, number[]>();
	const accepted: number[] = [];

	const insertBox = (index: number, box: LabelBox) => {
		const minCx = Math.floor(box.x * invCell);
		const maxCx = Math.floor((box.x + box.w) * invCell);
		const minCy = Math.floor(box.y * invCell);
		const maxCy = Math.floor((box.y + box.h) * invCell);

		for (let cy = minCy; cy <= maxCy; cy++) {
			for (let cx = minCx; cx <= maxCx; cx++) {
				const key = cellKey(cx, cy);
				const bucket = grid.get(key);
				if (bucket) {
					bucket.push(index);
				} else {
					grid.set(key, [index]);
				}
			}
		}
	};

	const collidesInGrid = (box: LabelBox): boolean => {
		const minCx = Math.floor(box.x * invCell);
		const maxCx = Math.floor((box.x + box.w) * invCell);
		const minCy = Math.floor(box.y * invCell);
		const maxCy = Math.floor((box.y + box.h) * invCell);

		for (let cy = minCy; cy <= maxCy; cy++) {
			for (let cx = minCx; cx <= maxCx; cx++) {
				const bucket = grid.get(cellKey(cx, cy));
				if (!bucket) continue;
				for (let k = 0; k < bucket.length; k++) {
					const otherIdx = bucket[k] as number;
					const otherBox = boxes[otherIdx] as LabelBox;
					if (boxesOverlap(box, otherBox)) {
						return true;
					}
				}
			}
		}
		return false;
	};

	for (let i = 0; i < boxes.length; i++) {
		const box = boxes[i] as LabelBox;
		if (box.w <= 0 || box.h <= 0) continue;

		const isForced = box.forced === true || forcedSet.has(i);
		if (isForced || !collidesInGrid(box)) {
			accepted.push(i);
			insertBox(i, box);
		}
	}

	return accepted;
}
