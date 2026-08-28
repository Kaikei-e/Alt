import { describe, expect, it } from "vitest";
import { BREAKPOINT_REM, isDesktop, isMobile } from "./viewport.svelte";

/**
 * Node has no `matchMedia`, so the reactive half of this module is exercised in
 * the browser project (`viewport.svelte.test.ts`), where the viewport can
 * actually be rotated. What is worth pinning here is the SSR answer: the server
 * renders without a window, and it must render the mobile layout rather than
 * guessing desktop and making every phone hydrate into a swap.
 */
describe("viewport (SSR / no window)", () => {
	it("exports the TailwindCSS `md` breakpoint", () => {
		expect(BREAKPOINT_REM).toBe(48);
	});

	it("falls back to mobile-first when there is no window to measure", () => {
		expect(isDesktop()).toBe(false);
		expect(isMobile()).toBe(true);
	});

	it("keeps the two sides complementary", () => {
		expect(isMobile()).toBe(!isDesktop());
	});
});
