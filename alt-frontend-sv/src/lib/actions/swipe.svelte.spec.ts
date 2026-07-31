/**
 * Swipe Action — browser-mode regression tests
 *
 * The `.svelte.` infix is what routes this file into vitest's `client`
 * project; nothing in here is a Svelte module. It has to run in a real
 * Chromium because the behaviour it pins — how the browser retargets a
 * `click` when an ancestor holds pointer capture — does not exist in jsdom,
 * and that blind spot is exactly how the regression below shipped.
 *
 * Regression being pinned: `swipe()` used to call `node.setPointerCapture()`
 * on every pointerdown. Chromium then delivers the follow-up compatibility
 * mouse events, and therefore the `click`, to the capturing element — so a
 * mouse- or pen-driven click on ANY control inside a swipe surface was
 * swallowed by the card and never reached the control. Touch taps are
 * synthesised from the tap gesture and hit-test the tap point, so they were
 * unaffected; that asymmetry is why the defect was invisible on a phone and
 * total in a desktop browser. It applied to every `use:swipe` surface
 * (SwipeFeedCard, VisualPreviewCard, SwipeRecapCard).
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { swipe } from "./swipe";

describe("swipe action", () => {
	let teardown: (() => void) | null = null;

	afterEach(() => {
		teardown?.();
		teardown = null;
	});

	/**
	 * A minimal stand-in for a swipe card: a surface carrying the action with
	 * an ordinary control inside it, which is the shape all three real swipe
	 * cards have.
	 */
	function mountSurface() {
		const surface = document.createElement("div");
		surface.dataset.testid = "swipe-surface";
		surface.style.cssText =
			"position:fixed;inset:0;display:flex;align-items:center;justify-content:center;touch-action:none;";

		const control = document.createElement("button");
		control.type = "button";
		control.dataset.testid = "swipe-control";
		control.textContent = "Article";
		control.style.cssText = "width:160px;height:48px;";

		surface.appendChild(control);
		document.body.appendChild(surface);

		const action = swipe(surface, { threshold: 50 });

		teardown = () => {
			action.destroy();
			surface.remove();
		};

		return { surface, control };
	}

	function pointerEvent(type: string, clientX: number, clientY: number) {
		return new PointerEvent(type, {
			clientX,
			clientY,
			pointerId: 1,
			pointerType: "mouse",
			bubbles: true,
			cancelable: true,
		});
	}

	it("delivers a click on a control inside the surface to that control", async () => {
		const { control } = mountSurface();

		const onControlClick = vi.fn();
		control.addEventListener("click", onControlClick);

		const clickTargets: string[] = [];
		const recordTarget = (ev: Event) => {
			clickTargets.push((ev.target as HTMLElement).dataset.testid ?? "unknown");
		};
		document.addEventListener("click", recordTarget, true);

		try {
			await page.getByTestId("swipe-control").click();
		} finally {
			document.removeEventListener("click", recordTarget, true);
		}

		expect(onControlClick).toHaveBeenCalledTimes(1);
		// If the surface holds pointer capture, the click is retargeted to it
		// and this reads ["swipe-surface"] with zero handler calls above.
		expect(clickTargets).toEqual(["swipe-control"]);
	});

	it("still tracks a gesture that begins on a control", async () => {
		const { surface, control } = mountSurface();

		const onMove = vi.fn();
		const onEnd = vi.fn();
		const onSwipeLeft = vi.fn();
		surface.addEventListener("swipe:move", onMove);
		surface.addEventListener("swipe:end", onEnd);
		surface.addEventListener("swipeleft", onSwipeLeft);

		// Press starts on the button, then drags across the card. Skipping
		// pointer capture must not make the action ignore the gesture: the
		// window-level pointermove/pointerup listeners are the primary
		// tracking mechanism and have to be armed regardless of the target.
		control.dispatchEvent(pointerEvent("pointerdown", 300, 200));
		window.dispatchEvent(pointerEvent("pointermove", 200, 205));
		await new Promise((resolve) => requestAnimationFrame(() => resolve(null)));
		window.dispatchEvent(pointerEvent("pointerup", 120, 205));

		expect(onMove).toHaveBeenCalled();
		expect(onEnd).toHaveBeenCalled();
		expect(onSwipeLeft).toHaveBeenCalled();
	});
});
