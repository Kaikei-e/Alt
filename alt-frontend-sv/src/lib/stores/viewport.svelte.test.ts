import { flushSync } from "svelte";
import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { BREAKPOINT, isDesktop, isMobile } from "./viewport.svelte";

// A Pixel 5 is 393 CSS px upright and 851 across in landscape; an iPhone 13 is
// 390 / 844. Both cross the 768px breakpoint just by being rotated, so
// "the viewport changed after the component mounted" is the ordinary case for
// a phone, not an edge case for someone dragging a desktop window.
const PORTRAIT = [393, 851] as const;
const LANDSCAPE = [851, 393] as const;

/** The media query change arrives as a browser event; give it a turn. */
async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
	flushSync();
}

/** Record every value an `$effect` observes for `read`, across the rotations. */
async function observeThroughRotation(read: () => boolean) {
	await page.viewport(...PORTRAIT);
	const seen: boolean[] = [];

	const stop = $effect.root(() => {
		$effect(() => {
			seen.push(read());
		});
	});
	flushSync();

	await page.viewport(...LANDSCAPE);
	await settle();
	await page.viewport(...PORTRAIT);
	await settle();
	stop();

	return seen;
}

describe("viewport", () => {
	it("exports the TailwindCSS `md` breakpoint", () => {
		expect(BREAKPOINT).toBe(768);
	});

	it("answers for the viewport it is asked in", async () => {
		await page.viewport(...PORTRAIT);
		expect(isDesktop()).toBe(false);
		expect(isMobile()).toBe(true);

		await page.viewport(...LANDSCAPE);
		expect(isDesktop()).toBe(true);
		expect(isMobile()).toBe(false);
	});

	// The defect this suite exists for. The old API was an object of getters
	// whose own docstring taught `const { isDesktop } = useViewport()`, and that
	// destructure read the getter once and bound a dead boolean: a phone opened
	// in landscape got the desktop layout and never left it. Anything that reads
	// the viewport from inside a reactive context has to see every change.
	it("re-runs an effect that reads it when the phone is rotated", async () => {
		expect(await observeThroughRotation(isDesktop)).toEqual([
			false,
			true,
			false,
		]);
	});

	it("re-runs an effect that reads the mobile side too", async () => {
		expect(await observeThroughRotation(isMobile)).toEqual([true, false, true]);
	});

	it("keeps the two sides complementary as the viewport moves", async () => {
		await page.viewport(...PORTRAIT);
		expect(isMobile()).toBe(!isDesktop());

		await page.viewport(...LANDSCAPE);
		expect(isMobile()).toBe(!isDesktop());
	});
});
