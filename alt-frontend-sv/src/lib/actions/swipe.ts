export type SwipeDirection = "left" | "right" | "up" | "down";

export interface SwipeOptions {
	// Minimum distance (px) to be considered a swipe
	threshold?: number;
	// Maximum perpendicular distance (px) allowed for a swipe
	restraint?: number;
	// Maximum time (ms) allowed for a swipe
	allowedTime?: number;
}

interface SwipeDetail {
	direction: SwipeDirection;
	deltaX: number;
	deltaY: number;
	elapsedTime: number;
	pointerType: string;
}

interface SwipeMoveDetail {
	deltaX: number;
	deltaY: number;
}

export function swipe(node: HTMLElement, options: SwipeOptions = {}) {
	let threshold = options.threshold ?? 80;
	let restraint = options.restraint ?? 120;
	let allowedTime = options.allowedTime ?? 500;

	let startX = 0;
	let startY = 0;
	let startTime = 0;
	let pointerId: number | null = null;
	let pointerType = "mouse";
	let active = false;

	let lastDx = 0;
	let lastDy = 0;
	let rafId = 0;

	function emitMove() {
		rafId = 0;
		if (!active) return;

		const detail: SwipeMoveDetail = { deltaX: lastDx, deltaY: lastDy };
		node.dispatchEvent(
			new CustomEvent<SwipeMoveDetail>("swipe:move", { detail }),
		);
	}

	function onPointerDown(ev: PointerEvent) {
		if (active) return;
		active = true;

		pointerId = ev.pointerId;
		pointerType = ev.pointerType;

		startX = ev.clientX;
		startY = ev.clientY;
		startTime = performance.now();

		lastDx = 0;
		lastDy = 0;

		try {
			node.setPointerCapture(ev.pointerId);
		} catch {
			// ignore
		}
	}

	function onPointerMove(ev: PointerEvent) {
		if (!active || ev.pointerId !== pointerId) return;

		lastDx = ev.clientX - startX;
		lastDy = ev.clientY - startY;

		if (!rafId) {
			rafId = requestAnimationFrame(emitMove);
		}
	}

	function endPointer(ev: PointerEvent) {
		if (!active || ev.pointerId !== pointerId) return;

		active = false;

		if (rafId) {
			cancelAnimationFrame(rafId);
			rafId = 0;
		}

		const dx = ev.clientX - startX;
		const dy = ev.clientY - startY;
		const elapsed = performance.now() - startTime;

		// ★ 時間に関係なく「必ず」 swipe:end を飛ばす
		const endDetail: SwipeMoveDetail = { deltaX: dx, deltaY: dy };
		node.dispatchEvent(
			new CustomEvent<SwipeMoveDetail>("swipe:end", { detail: endDetail }),
		);

		// ここからは「スワイプ成立」の判定
		let direction: SwipeDirection | null = null;

		if (elapsed <= allowedTime) {
			if (Math.abs(dx) >= threshold && Math.abs(dy) <= restraint) {
				direction = dx > 0 ? "right" : "left";
			} else if (Math.abs(dy) >= threshold && Math.abs(dx) <= restraint) {
				direction = dy > 0 ? "down" : "up";
			}
		}

		if (direction) {
			const detail: SwipeDetail = {
				direction,
				deltaX: dx,
				deltaY: dy,
				elapsedTime: elapsed,
				pointerType,
			};
			node.dispatchEvent(new CustomEvent<SwipeDetail>("swipe", { detail }));

			// Direction-specific events
			node.dispatchEvent(
				new CustomEvent<SwipeDirection>(`swipe${direction}`, {
					detail: direction,
				}),
			);
		}

		try {
			node.releasePointerCapture(ev.pointerId);
		} catch {
			// ignore
		}
		pointerId = null;
	}

	function onPointerUp(ev: PointerEvent) {
		endPointer(ev);
	}

	function onPointerCancel(ev: PointerEvent) {
		endPointer(ev);
	}

	node.addEventListener("pointerdown", onPointerDown);
	node.addEventListener("pointermove", onPointerMove);
	node.addEventListener("pointerup", onPointerUp);
	node.addEventListener("pointercancel", onPointerCancel);
	node.addEventListener("pointerleave", onPointerCancel);

	return {
		update(newOptions: SwipeOptions) {
			threshold = newOptions.threshold ?? threshold;
			restraint = newOptions.restraint ?? restraint;
			allowedTime = newOptions.allowedTime ?? allowedTime;
		},
		destroy() {
			node.removeEventListener("pointerdown", onPointerDown);
			node.removeEventListener("pointermove", onPointerMove);
			node.removeEventListener("pointerup", onPointerUp);
			node.removeEventListener("pointercancel", onPointerCancel);
			node.removeEventListener("pointerleave", onPointerCancel);
			if (rafId) cancelAnimationFrame(rafId);
		},
	};
}
