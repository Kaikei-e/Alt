/**
 * Swipe Action Tests
 *
 * Tests for the swipe gesture action using pointer events
 *
 * @vitest-environment jsdom
 */
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { swipe, type SwipeDirection } from "./swipe";

describe("swipe action", () => {
	let element: HTMLDivElement;

	beforeEach(() => {
		element = document.createElement("div");
		document.body.appendChild(element);
	});

	afterEach(() => {
		document.body.removeChild(element);
	});

	// Helper to create pointer events
	function createPointerEvent(
		type: string,
		options: {
			clientX: number;
			clientY: number;
			pointerId?: number;
			pointerType?: string;
		},
	): PointerEvent {
		return new PointerEvent(type, {
			clientX: options.clientX,
			clientY: options.clientY,
			pointerId: options.pointerId ?? 1,
			pointerType: options.pointerType ?? "mouse",
			bubbles: true,
			cancelable: true,
		});
	}

	it("should initialize without errors", () => {
		const action = swipe(element);
		expect(action).toBeDefined();
		expect(action.update).toBeDefined();
		expect(action.destroy).toBeDefined();
	});

	it("should emit swipe:move event during pointer move", async () => {
		const moveHandler = vi.fn();
		element.addEventListener("swipe:move", moveHandler as EventListener);

		swipe(element);

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);

		// Wait for next animation frame
		await new Promise((resolve) => requestAnimationFrame(resolve));

		// Simulate pointer move
		window.dispatchEvent(
			createPointerEvent("pointermove", { clientX: 50, clientY: 0 }),
		);

		// Wait for requestAnimationFrame
		await new Promise((resolve) => requestAnimationFrame(resolve));
		await new Promise((resolve) => requestAnimationFrame(resolve));

		expect(moveHandler).toHaveBeenCalled();
	});

	it("should emit swipe:end event on pointer up", async () => {
		const endHandler = vi.fn();
		element.addEventListener("swipe:end", endHandler as EventListener);

		swipe(element);

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);

		// Simulate pointer up
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 50, clientY: 0 }),
		);

		expect(endHandler).toHaveBeenCalled();
	});

	it("should emit swipe event with direction 'right' for right swipe", async () => {
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		swipe(element, { threshold: 50 });

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);

		// Simulate pointer up with enough distance
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 100, clientY: 0 }),
		);

		expect(swipeHandler).toHaveBeenCalled();
		const detail = (swipeHandler.mock.calls[0][0] as CustomEvent).detail;
		expect(detail.direction).toBe("right");
		expect(detail.deltaX).toBe(100);
	});

	it("should emit swipe event with direction 'left' for left swipe", async () => {
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		swipe(element, { threshold: 50 });

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 100, clientY: 0 }),
		);

		// Simulate pointer up with enough distance
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 0, clientY: 0 }),
		);

		expect(swipeHandler).toHaveBeenCalled();
		const detail = (swipeHandler.mock.calls[0][0] as CustomEvent).detail;
		expect(detail.direction).toBe("left");
		expect(detail.deltaX).toBe(-100);
	});

	it("should emit swipe event with direction 'down' for down swipe", async () => {
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		swipe(element, { threshold: 50 });

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);

		// Simulate pointer up with enough distance
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 0, clientY: 100 }),
		);

		expect(swipeHandler).toHaveBeenCalled();
		const detail = (swipeHandler.mock.calls[0][0] as CustomEvent).detail;
		expect(detail.direction).toBe("down");
		expect(detail.deltaY).toBe(100);
	});

	it("should emit swipe event with direction 'up' for up swipe", async () => {
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		swipe(element, { threshold: 50 });

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 100 }),
		);

		// Simulate pointer up with enough distance
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 0, clientY: 0 }),
		);

		expect(swipeHandler).toHaveBeenCalled();
		const detail = (swipeHandler.mock.calls[0][0] as CustomEvent).detail;
		expect(detail.direction).toBe("up");
		expect(detail.deltaY).toBe(-100);
	});

	it("should not emit swipe event if distance is below threshold", async () => {
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		swipe(element, { threshold: 100 });

		// Simulate pointer down
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);

		// Simulate pointer up with not enough distance
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 50, clientY: 0 }),
		);

		expect(swipeHandler).not.toHaveBeenCalled();
	});

	it("should update options via update method", () => {
		const action = swipe(element, { threshold: 50 });

		// Update should work without errors
		action.update({ threshold: 100 });

		expect(action).toBeDefined();
	});

	it("should cleanup listeners on destroy", () => {
		const action = swipe(element);

		// Destroy should work without errors
		action.destroy();

		// After destroy, events should not trigger swipe
		const swipeHandler = vi.fn();
		element.addEventListener("swipe", swipeHandler as EventListener);

		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 100, clientY: 0 }),
		);

		expect(swipeHandler).not.toHaveBeenCalled();
	});

	it("should emit direction-specific event (swiperight)", async () => {
		const swipeRightHandler = vi.fn();
		element.addEventListener("swiperight", swipeRightHandler as EventListener);

		swipe(element, { threshold: 50 });

		// Simulate right swipe
		element.dispatchEvent(
			createPointerEvent("pointerdown", { clientX: 0, clientY: 0 }),
		);
		window.dispatchEvent(
			createPointerEvent("pointerup", { clientX: 100, clientY: 0 }),
		);

		expect(swipeRightHandler).toHaveBeenCalled();
	});
});
