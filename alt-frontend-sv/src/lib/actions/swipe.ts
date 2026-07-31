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

// Controls a press can legitimately start on inside a swipe surface. Pointer
// capture must not be taken for these — see onPointerDown below.
const INTERACTIVE_SELECTOR =
	"button, a[href], input, select, textarea, label, [role='button'], [role='link'], [contenteditable=''], [contenteditable='true']";

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
	let touchStartX = 0;
	let touchStartY = 0;
	let touchActive = false;
	let touchAxis: "undecided" | "x" | "y" = "undecided";

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

		// Androidでも確実にイベントを受け取るため、windowレベルでキャッチ
		// setPointerCaptureはAndroidで不安定なことがあるため、フォールバックとして使用
		//
		// ただしカード内のコントロール上で始まった押下では capture を取らない:
		// capture 中の要素には後続の click が retarget されるため、カード自身が
		// click を飲み込んでしまい、ボタンの onclick が一度も発火しなくなる
		// （マウス / ペンのみ。タッチの tap は tap 位置で hit-test されるため
		// 影響を受けず、これがこの不具合がモバイルで見えなかった理由）。
		// 下の window レベル pointermove/pointerup が本来の追跡経路なので、
		// ここで capture を省いてもジェスチャの精度は落ちない。
		const target = ev.target;
		const startedOnControl =
			target instanceof Element &&
			target.closest(INTERACTIVE_SELECTOR) !== null;

		if (!startedOnControl) {
			try {
				node.setPointerCapture(ev.pointerId);
			} catch {
				// ignore
			}
		}

		// windowレベルでpointermove/pointerupをキャッチ（Android対策）
		window.addEventListener("pointermove", onPointerMove);
		window.addEventListener("pointerup", onPointerUp);
		window.addEventListener("pointercancel", onPointerCancel);
	}

	function onTouchStart(ev: TouchEvent) {
		// Pointer Events が先に active になっていても、Android では touchmove 側で
		// preventDefault() しないとスクロール判定→pointercancel でスワイプが途切れることがある。
		// そのため active の有無では弾かない。
		if (touchActive) return;

		const touch = ev.touches[0];
		if (!touch) return;

		touchActive = true;
		touchStartX = touch.clientX;
		touchStartY = touch.clientY;
		touchAxis = "undecided";
	}

	function onTouchMove(ev: TouchEvent) {
		if (!touchActive) return;

		const touch = ev.touches[0];
		if (!touch) return;

		const dx = touch.clientX - touchStartX;
		const dy = touch.clientY - touchStartY;

		const adx = Math.abs(dx);
		const ady = Math.abs(dy);

		// 軸ロック（早めに決める）。決まったらその軸に従う。
		// ※小さな揺れで毎フレーム切り替わるとUXが悪いので、最初に決めたら固定する。
		if (touchAxis === "undecided") {
			const LOCK_THRESHOLD_PX = 8;
			if (adx < LOCK_THRESHOLD_PX && ady < LOCK_THRESHOLD_PX) return;
			touchAxis = adx > ady ? "x" : "y";
		}

		// 横スワイプと判定した場合のみスクロールを阻止（Androidでpointercancelを防ぐ）
		if (touchAxis === "x") {
			ev.preventDefault();
		}
	}

	function onTouchEnd(_ev: TouchEvent) {
		touchActive = false;
		touchAxis = "undecided";
	}

	function onPointerMove(ev: PointerEvent) {
		if (!active || ev.pointerId !== pointerId) return;

		lastDx = ev.clientX - startX;
		lastDy = ev.clientY - startY;

		if (!rafId) {
			rafId = requestAnimationFrame(emitMove);
		}
	}

	function cleanupWindowListeners() {
		window.removeEventListener("pointermove", onPointerMove);
		window.removeEventListener("pointerup", onPointerUp);
		window.removeEventListener("pointercancel", onPointerCancel);
	}

	function cleanupTouchListeners() {
		node.removeEventListener("touchstart", onTouchStart);
		node.removeEventListener("touchmove", onTouchMove);
		node.removeEventListener("touchend", onTouchEnd);
		node.removeEventListener("touchcancel", onTouchEnd);
	}

	function endPointer(ev: PointerEvent) {
		if (!active || ev.pointerId !== pointerId) return;

		active = false;

		// windowレベルのリスナーをクリーンアップ
		cleanupWindowListeners();

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

		// ここからは「スワイプ成立」の判定（距離だけで判定）
		let direction: SwipeDirection | null = null;

		if (Math.abs(dx) >= threshold && Math.abs(dy) <= restraint) {
			direction = dx > 0 ? "right" : "left";
		} else if (Math.abs(dy) >= threshold && Math.abs(dx) <= restraint) {
			direction = dy > 0 ? "down" : "up";
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

	// pointerdownのみnodeに登録（pointermove/upはpointerdown時にwindowに登録）
	node.addEventListener("pointerdown", onPointerDown);

	// touchイベントを追加（Androidでpointercancelを防ぐため）
	// passive: false で登録（preventDefaultを呼ぶため必須）
	// capture: true で早めに拾い、スクロール開始より前に preventDefault できる確率を上げる
	node.addEventListener("touchstart", onTouchStart, {
		passive: true,
		capture: true,
	});
	node.addEventListener("touchmove", onTouchMove, {
		passive: false,
		capture: true,
	});
	node.addEventListener("touchend", onTouchEnd, {
		passive: true,
		capture: true,
	});
	node.addEventListener("touchcancel", onTouchEnd, {
		passive: true,
		capture: true,
	});

	return {
		update(newOptions: SwipeOptions) {
			threshold = newOptions.threshold ?? threshold;
			restraint = newOptions.restraint ?? restraint;
			allowedTime = newOptions.allowedTime ?? allowedTime;
		},
		destroy() {
			node.removeEventListener("pointerdown", onPointerDown);
			cleanupWindowListeners();
			cleanupTouchListeners();
			if (rafId) cancelAnimationFrame(rafId);
		},
	};
}
