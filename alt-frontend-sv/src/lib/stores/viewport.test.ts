import { describe, expect, it } from "vitest";
import { BREAKPOINT, createViewportState } from "./viewport.svelte";

/**
 * Note: MediaQuery from svelte/reactivity requires a browser environment
 * and cannot be unit-tested in Node. We test the exported contract:
 * - useViewport() returns { isDesktop, isMobile } (reactive getters)
 * - BREAKPOINT constant is exported and equals 768
 * - createViewportState() creates an object with correct defaults
 */

describe("Viewport Store", () => {
	describe("BREAKPOINT", () => {
		it("should export BREAKPOINT as 768", () => {
			expect(BREAKPOINT).toBe(768);
		});
	});

	describe("createViewportState", () => {
		it("should return an object with isDesktop and isMobile properties", () => {
			const state = createViewportState();

			expect(state).toHaveProperty("isDesktop");
			expect(state).toHaveProperty("isMobile");
		});

		it("should default to mobile (isDesktop=false, isMobile=true) for SSR fallback", () => {
			const state = createViewportState();

			// In Node (non-browser), MediaQuery falls back to false for min-width
			// so isDesktop should be false and isMobile should be true
			expect(state.isDesktop).toBe(false);
			expect(state.isMobile).toBe(true);
		});

		it("should return complementary values (isDesktop and isMobile are opposite)", () => {
			const state = createViewportState();

			expect(state.isDesktop).toBe(!state.isMobile);
		});
	});
});
