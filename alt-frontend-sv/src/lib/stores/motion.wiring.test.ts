/**
 * The reactive wiring of the motion store, against a file-scoped mock of
 * `svelte/reactivity`.
 *
 * Lives apart from motion.test.ts because that file needs the REAL module (it
 * pins the SSR fallback), and a hoisted `vi.mock` covers a whole file. The
 * previous version of this spec lived there and mocked per-test with
 * `vi.resetModules()` + `vi.doMock` + `Promise.all` dynamic imports — and
 * vitest does not guarantee a doMock factory runs only once under concurrent
 * imports, so the two importers could each get their own copy of the mock
 * class and the test's instance registry stayed empty (1-in-30 locally,
 * reliably on CI). Here the mock is hoisted before any import, the import is
 * static, and the mock's state lives in one `vi.hoisted` object closed over
 * by the factory, so even a double factory run cannot split it.
 */
import { describe, expect, it, vi } from "vitest";

const mediaQueryState = vi.hoisted(() => ({
	query: null as string | null,
	fallback: null as boolean | null,
	constructed: 0,
	current: false,
}));

vi.mock("svelte/reactivity", () => ({
	MediaQuery: class MockMediaQuery {
		constructor(query: string, fallback?: boolean) {
			mediaQueryState.constructed++;
			mediaQueryState.query = query;
			mediaQueryState.fallback = fallback ?? null;
			mediaQueryState.current = fallback ?? false;
		}
		get current(): boolean {
			return mediaQueryState.current;
		}
	},
}));

import { prefersReducedMotion } from "./motion.svelte";

describe("motion (media query wiring)", () => {
	it("subscribes once to prefers-reduced-motion with the motion-first fallback", () => {
		expect(mediaQueryState.constructed).toBe(1);
		expect(mediaQueryState.query).toBe("prefers-reduced-motion: reduce");
		expect(mediaQueryState.fallback).toBe(false);
	});

	it("follows the media query", () => {
		mediaQueryState.current = false;
		expect(prefersReducedMotion()).toBe(false);

		mediaQueryState.current = true;
		expect(prefersReducedMotion()).toBe(true);

		mediaQueryState.current = false;
		expect(prefersReducedMotion()).toBe(false);
	});
});
