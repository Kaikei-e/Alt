/**
 * DispatchHeader Component Tests
 *
 * Editorial mini-header for the immersive swipe dispatch surfaces:
 * back exit, kicker masthead, session counter, undo, filter trigger.
 */

import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import DispatchHeader from "./DispatchHeader.svelte";

const baseProps = {
	mode: "visual-preview" as const,
	readCount: 0,
	canUndo: false,
	onUndo: () => {},
	onOpenFilter: () => {},
	filterActive: false,
};

describe("DispatchHeader", () => {
	it("shows the PHOTO WIRE kicker in visual-preview mode", async () => {
		const screen = render(DispatchHeader, { ...baseProps });
		await expect
			.element(screen.getByTestId("dispatch-kicker"))
			.toHaveTextContent("Photo Wire");
	});

	it("shows the WIRE DISPATCH kicker in default mode", async () => {
		const screen = render(DispatchHeader, { ...baseProps, mode: "default" });
		await expect
			.element(screen.getByTestId("dispatch-kicker"))
			.toHaveTextContent("Wire Dispatch");
	});

	it("shows the session counter", async () => {
		const screen = render(DispatchHeader, { ...baseProps, readCount: 14 });
		await expect
			.element(screen.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 14");
	});

	it("exits to the menu page", async () => {
		const screen = render(DispatchHeader, { ...baseProps });
		await expect
			.element(screen.getByTestId("dispatch-back"))
			.toHaveAttribute("href", "/menu");
	});

	it("disables undo when there is nothing to undo", async () => {
		const screen = render(DispatchHeader, { ...baseProps, canUndo: false });
		await expect.element(screen.getByTestId("dispatch-undo")).toBeDisabled();
	});

	it("calls onUndo when undo is tapped", async () => {
		const onUndo = vi.fn();
		const screen = render(DispatchHeader, {
			...baseProps,
			canUndo: true,
			onUndo,
		});
		await screen.getByTestId("dispatch-undo").click();
		expect(onUndo).toHaveBeenCalledOnce();
	});

	it("opens the filter sheet from the header trigger", async () => {
		const onOpenFilter = vi.fn();
		const screen = render(DispatchHeader, { ...baseProps, onOpenFilter });
		await screen.getByTestId("swipe-filter-trigger").click();
		expect(onOpenFilter).toHaveBeenCalledOnce();
	});

	it("marks the filter trigger when a filter is active", async () => {
		const screen = render(DispatchHeader, { ...baseProps, filterActive: true });
		await expect
			.element(screen.getByTestId("filter-active-badge"))
			.toBeInTheDocument();
	});
});
