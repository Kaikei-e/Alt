/**
 * Client hook tests
 *
 * SvelteKit emits dynamic-import / chunk-load failures through
 * `handleError`. When the deploy rotates `_app/immutable/*` hashes, an
 * already-open tab on iOS Safari fails the next chunk fetch with a
 * generic "Failed to fetch dynamically imported module" or similar. The
 * client hook must catch that family and force a full reload so the
 * browser pulls the fresh HTML + manifest pair.
 */

import { describe, expect, it, vi, beforeEach } from "vitest";
import {
	createChunkReloadScheduler,
	isChunkLoadError,
	buildClientErrorPayload,
} from "./hooks.client";

describe("isChunkLoadError", () => {
	it("matches SvelteKit dynamic import failure", () => {
		expect(
			isChunkLoadError(
				"Failed to fetch dynamically imported module: /_app/immutable/chunks/abc.js",
			),
		).toBe(true);
	});

	it("matches Vite ChunkLoadError", () => {
		expect(isChunkLoadError("ChunkLoadError: Loading chunk 42 failed")).toBe(
			true,
		);
	});

	it("matches webpack-style Loading chunk failed", () => {
		expect(isChunkLoadError("Loading chunk 7 failed")).toBe(true);
	});

	it("matches module script load failure (Safari)", () => {
		expect(isChunkLoadError("Importing a module script failed")).toBe(true);
		expect(isChunkLoadError("Failed to load module script: foo")).toBe(true);
	});

	it("does not match unrelated errors", () => {
		expect(isChunkLoadError("TypeError: x is undefined")).toBe(false);
		expect(
			isChunkLoadError("NetworkError when attempting to fetch resource."),
		).toBe(false);
		expect(isChunkLoadError("")).toBe(false);
	});
});

describe("createChunkReloadScheduler", () => {
	let reload: ReturnType<typeof vi.fn<() => void>>;
	let storage: Map<string, string>;
	const fakeStorage = (): Storage =>
		({
			getItem: (k: string) => storage.get(k) ?? null,
			setItem: (k: string, v: string) => {
				storage.set(k, v);
			},
			removeItem: (k: string) => {
				storage.delete(k);
			},
			clear: () => {
				storage.clear();
			},
			key: () => null,
			length: 0,
		}) as Storage;

	beforeEach(() => {
		reload = vi.fn<() => void>();
		storage = new Map();
	});

	it("calls reload exactly once even when scheduled multiple times", () => {
		const s = createChunkReloadScheduler({ reload, storage: fakeStorage() });
		s.schedule("chunk-404");
		s.schedule("chunk-404");
		s.schedule("updated");
		expect(reload).toHaveBeenCalledTimes(1);
	});

	it("stops reloading after 3 attempts within the session", () => {
		const s = createChunkReloadScheduler({ reload, storage: fakeStorage() });
		// Simulate 3 prior reload attempts already counted.
		storage.set("alt:chunk-reload-attempts", "3");
		s.schedule("chunk-404");
		expect(reload).not.toHaveBeenCalled();
	});

	it("increments the attempt counter on each schedule that fires", () => {
		const s1 = createChunkReloadScheduler({ reload, storage: fakeStorage() });
		s1.schedule("chunk-404");
		expect(storage.get("alt:chunk-reload-attempts")).toBe("1");
	});

	it("tolerates missing storage (best-effort reload)", () => {
		const s = createChunkReloadScheduler({ reload });
		s.schedule("updated");
		expect(reload).toHaveBeenCalledTimes(1);
	});
});

describe("buildClientErrorPayload", () => {
	it("classifies iOS Safari UA", () => {
		const ua =
			"Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.7 Mobile/15E148 Safari/604.1";
		const p = buildClientErrorPayload({
			error: new Error(
				"Failed to fetch dynamically imported module: /_app/immutable/chunks/abc-def123.js",
			),
			path: "/feeds",
			status: 500,
			message: "fail",
			userAgent: ua,
		});
		expect(p.safariBucket).toBe("ios-safari");
		expect(p.chunkHash).toBe("abc-def123");
	});

	it("classifies macOS Safari UA", () => {
		const ua =
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Safari/605.1.15";
		const p = buildClientErrorPayload({
			error: new Error("noop"),
			path: "/",
			status: 500,
			message: "fail",
			userAgent: ua,
		});
		expect(p.safariBucket).toBe("macos-safari");
		expect(p.chunkHash).toBeUndefined();
	});

	it("classifies non-Safari UA as other", () => {
		const ua =
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0 Safari/537.36";
		const p = buildClientErrorPayload({
			error: new Error("noop"),
			path: "/",
			status: 500,
			message: "fail",
			userAgent: ua,
		});
		expect(p.safariBucket).toBe("other");
	});

	it("handles missing user agent", () => {
		const p = buildClientErrorPayload({
			error: new Error("noop"),
			path: "/",
			status: 500,
			message: "fail",
			userAgent: undefined,
		});
		expect(p.safariBucket).toBe("other");
	});
});
