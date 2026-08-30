import { describe, expect, it } from "vitest";
import { prefersReducedMotion } from "./motion.svelte";

/**
 * Node has no `matchMedia`, so what the real server build answers is the SSR
 * fallback — and it must be false: motion is the default, reduction the
 * explicit request. The reactive wiring is pinned in motion.wiring.test.ts
 * against a file-scoped mock of `svelte/reactivity` (this file must keep the
 * real module, so the two cannot share a file), and end-to-end through the
 * reduced-motion component spec in the client project.
 */
describe("motion (SSR / no window)", () => {
	it("falls back to full motion when there is no window to ask", () => {
		expect(prefersReducedMotion()).toBe(false);
	});
});
