export interface LabelBox {
	x: number;
	y: number;
	w: number;
	h: number;
	forced?: boolean;
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

export function layoutLabels(
	boxes: readonly LabelBox[],
	cellSize = DEFAULT_CELL_SIZE,
): number[] {
	if (boxes.length === 0) return [];

	const resolvedCellSize = cellSize > 0 ? cellSize : DEFAULT_CELL_SIZE;
	const invCell = 1 / resolvedCellSize;
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

		if (box.forced === true || !collidesInGrid(box)) {
			accepted.push(i);
			insertBox(i, box);
		}
	}

	return accepted;
}
