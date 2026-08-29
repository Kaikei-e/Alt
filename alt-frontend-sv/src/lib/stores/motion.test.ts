import { describe, expect, it, vi } from "vitest";
import { prefersReducedMotion } from "./motion.svelte";

/**
 * Node has no `matchMedia`, so what the real server build answers is the SSR
 * fallback — and it must be false: motion is the default, reduction the
 * explicit request. The reactive half is exercised below against a mocked
 * `svelte/reactivity` (this project runs in node), and end-to-end through the
 * reduced-motion component spec in the client project.
 */
describe("motion (SSR / no window)", () => {
	it("falls back to full motion when there is no window to ask", () => {
		expect(prefersReducedMotion()).toBe(false);
	});
});

describe("motion (media query wiring)", () => {
	it("follows the media query", async () => {
		vi.resetModules();
		vi.doMock("svelte/reactivity", () => {
			class FakeMediaQuery {
				current = false;
				readonly query: string;
				constructor(query: string, fallback?: boolean) {
					this.query = query;
					this.current = fallback ?? false;
					FakeMediaQuery.instances.push(this);
				}
				static instances: FakeMediaQuery[] = [];
			}
			return { MediaQuery: FakeMediaQuery };
		});

		const [{ prefersReducedMotion: fresh }, { MediaQuery: Fake }] =
			await Promise.all([
				import("./motion.svelte"),
				import("svelte/reactivity"),
			]);
		const instances = (
			Fake as unknown as {
				instances: { current: boolean; query: string }[];
			}
		).instances;

		expect(instances).toHaveLength(1);
		expect(instances[0]?.query).toBe("prefers-reduced-motion: reduce");
		expect(fresh()).toBe(false);

		instances[0]!.current = true;
		expect(fresh()).toBe(true);

		instances[0]!.current = false;
		expect(fresh()).toBe(false);

		vi.doUnmock("svelte/reactivity");
		vi.resetModules();
	});
});
