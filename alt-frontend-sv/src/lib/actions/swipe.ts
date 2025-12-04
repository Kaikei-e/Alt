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
}

export function swipe(node: HTMLElement, options: SwipeOptions = {}) {
	const threshold = options.threshold ?? 50;
	const restraint = options.restraint ?? 100;
	const allowedTime = options.allowedTime ?? 500;

	let startX = 0;
	let startY = 0;
	let startTime = 0;
	let pointerId: number | null = null;
	let isPointerDown = false;

	function onPointerDown(event: PointerEvent) {
		isPointerDown = true;
		pointerId = event.pointerId;

		startX = event.clientX;
		startY = event.clientY;
		startTime = Date.now();

		node.setPointerCapture(pointerId);
	}

	function onPointerMove(_event: PointerEvent) {
		// Optional: Add logic here if you want to track movement during drag
	}

	function onPointerEnd(event: PointerEvent) {
		if (!isPointerDown) return;

		const endX = event.clientX;
		const endY = event.clientY;
		const distX = endX - startX;
		const distY = endY - startY;
		const elapsed = Date.now() - startTime;

		isPointerDown = false;

		if (pointerId !== null) {
			try {
				node.releasePointerCapture(pointerId);
			} catch {
				// Ignore if already released
			}
			pointerId = null;
		}

		// Time constraint
		if (elapsed > allowedTime) return;

		let direction: SwipeDirection | null = null;

		// Horizontal swipe
		if (Math.abs(distX) >= threshold && Math.abs(distY) <= restraint) {
			direction = distX > 0 ? "right" : "left";
		}
		// Vertical swipe
		else if (Math.abs(distY) >= threshold && Math.abs(distX) <= restraint) {
			direction = distY > 0 ? "down" : "up";
		}

		if (!direction) return;

		const detail: SwipeDetail = {
			direction,
			deltaX: distX,
			deltaY: distY,
		};

		// Generic swipe event
		node.dispatchEvent(new CustomEvent<SwipeDetail>("swipe", { detail }));

		// Direction-specific events
		node.dispatchEvent(
			new CustomEvent<SwipeDirection>(`swipe${direction}`, {
				detail: direction,
			}),
		);
	}

	function onPointerCancel(event: PointerEvent) {
		onPointerEnd(event);
	}

	node.addEventListener("pointerdown", onPointerDown);
	node.addEventListener("pointermove", onPointerMove);
	node.addEventListener("pointerup", onPointerEnd);
	node.addEventListener("pointercancel", onPointerCancel);
	node.addEventListener("pointerleave", onPointerCancel);

	return {
		update(_newOptions: SwipeOptions) {
			// Update options if needed
		},
		destroy() {
			node.removeEventListener("pointerdown", onPointerDown);
			node.removeEventListener("pointermove", onPointerMove);
			node.removeEventListener("pointerup", onPointerEnd);
			node.removeEventListener("pointercancel", onPointerCancel);
			node.removeEventListener("pointerleave", onPointerCancel);
		},
	};
}
