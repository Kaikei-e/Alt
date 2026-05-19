/**
 * Tests for the iOS Safari chunk-404 bootstrap script.
 *
 * The bootstrap runs as inline JavaScript inside `app.html` before any
 * SvelteKit runtime chunk is evaluated, so the recovery still fires when
 * `_app/immutable/entry/app.<HASH>.js` itself returns 404 — the failure
 * mode ADR-000898's `hooks.client.ts` cannot catch (because hooks.client
 * is *inside* the entry chunk that failed to load).
 *
 * The exported `installChunkBootstrap` is the testable form of the same
 * logic that lives inline in `app.html`; a drift test asserts that the
 * inline copy in `app.html` matches the canonical body modulo whitespace.
 */

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
	BOOTSTRAP_SCRIPT_BODY,
	installChunkBootstrap,
} from "./safari-error-bootstrap";

interface BootstrapHarness {
	window: Window;
	reload: ReturnType<typeof vi.fn>;
	scheduleReload: ReturnType<typeof vi.fn>;
	storage: Map<string, string>;
	cleanup: () => void;
}

function makeStorage(seed?: number): {
	storage: Map<string, string>;
	api: Storage;
} {
	const storage = new Map<string, string>();
	if (typeof seed === "number") {
		storage.set("alt:chunk-reload-attempts", String(seed));
	}
	const api: Storage = {
		getItem: (k: string) => storage.get(k) ?? null,
		setItem: (k: string, v: string) => {
			storage.set(k, v);
		},
		removeItem: (k: string) => {
			storage.delete(k);
		},
		clear: () => storage.clear(),
		key: () => null,
		get length() {
			return storage.size;
		},
	};
	return { storage, api };
}

function makeHarness(initialAttempts?: number): BootstrapHarness {
	const { storage, api: storageApi } = makeStorage(initialAttempts);
	const eventTarget = new EventTarget();
	const win = Object.assign(eventTarget, {
		sessionStorage: storageApi,
		setTimeout: (cb: () => void) => {
			cb();
			return 0 as unknown as ReturnType<typeof setTimeout>;
		},
		location: { reload: () => {} },
	}) as unknown as Window;
	const reload = vi.fn();
	const scheduleReload = vi.fn((cb: () => void) => {
		cb();
		return 0;
	});
	const cleanup = installChunkBootstrap({
		window: win,
		reload,
		scheduleReload,
	});
	return { window: win, reload, scheduleReload, storage, cleanup };
}

function fireScriptError(win: Window, target: { src?: string; href?: string }) {
	const event = new Event("error", { bubbles: false });
	Object.defineProperty(event, "target", { value: target });
	win.dispatchEvent(event);
}

describe("installChunkBootstrap", () => {
	let harness: BootstrapHarness;

	beforeEach(() => {
		harness = makeHarness();
	});

	it("reloads when a /_app/immutable/* <script> fails to load (capture phase)", () => {
		fireScriptError(harness.window, {
			src: "/_app/immutable/entry/app.CITyonVd.js",
		});
		expect(harness.reload).toHaveBeenCalledTimes(1);
		expect(harness.storage.get("alt:chunk-reload-attempts")).toBe("1");
	});

	it("ignores errors on unrelated resources (e.g. random <link>)", () => {
		fireScriptError(harness.window, { href: "/static/some-style.css" });
		expect(harness.reload).not.toHaveBeenCalled();
	});

	it("coalesces multiple chunk-404 errors in the same navigation into a single reload", () => {
		const sources = [
			"/_app/immutable/entry/app.CITyonVd.js",
			"/_app/immutable/entry/start.Bbu4jThz.js",
			"/_app/immutable/nodes/0.BVcvTbxj.js",
			"/_app/immutable/chunks/BQkBW9PU.js",
			"/_app/immutable/chunks/BrrFf79v.js",
			"/_app/immutable/chunks/CrHF9HLM.js",
			"/_app/immutable/nodes/2.p2yQKcZ3.js",
			"/_app/immutable/nodes/29.DBlgLx6D.js",
		];
		for (const src of sources) {
			fireScriptError(harness.window, { src });
		}
		expect(harness.reload).toHaveBeenCalledTimes(1);
		expect(harness.storage.get("alt:chunk-reload-attempts")).toBe("1");
	});

	it("reloads on the Vite `vite:preloadError` custom event", () => {
		harness.window.dispatchEvent(new Event("vite:preloadError"));
		expect(harness.reload).toHaveBeenCalledTimes(1);
	});

	it("respects the 3-attempt reload ceiling and skips when already at limit", () => {
		harness.cleanup();
		harness = makeHarness(3);
		fireScriptError(harness.window, {
			src: "/_app/immutable/entry/app.x.js",
		});
		expect(harness.reload).not.toHaveBeenCalled();
		expect(harness.storage.get("alt:chunk-reload-attempts")).toBe("3");
	});

	it("removes both listeners on cleanup so they do not leak across navigations", () => {
		const target = harness.window as unknown as EventTarget;
		const spy = vi.spyOn(target, "removeEventListener");
		harness.cleanup();
		const types = spy.mock.calls.map((c) => c[0]);
		expect(types).toContain("error");
		expect(types).toContain("vite:preloadError");
	});
});

describe("BOOTSTRAP_SCRIPT_BODY in app.html (drift test)", () => {
	const normalize = (src: string): string => src.replace(/\s+/g, "");

	it("is embedded inside app.html so the inline copy cannot drift from the testable function", () => {
		const appHtmlPath = resolve(process.cwd(), "src/app.html");
		const appHtml = readFileSync(appHtmlPath, "utf8");
		expect(appHtml).toContain("<!-- alt:chunk-bootstrap:begin -->");
		expect(appHtml).toContain("<!-- alt:chunk-bootstrap:end -->");
		const block = appHtml.match(
			/<!-- alt:chunk-bootstrap:begin -->([\s\S]*?)<!-- alt:chunk-bootstrap:end -->/,
		);
		expect(block).not.toBeNull();
		const scriptInner =
			block?.[1].match(/<script>([\s\S]*?)<\/script>/)?.[1] ?? "";
		// Whitespace-normalized comparison — biome / prettier can reformat
		// either copy without breaking the test, but any semantic change
		// (different identifier names, dropped guards, changed limit, …)
		// will mismatch.
		expect(normalize(scriptInner)).toBe(normalize(BOOTSTRAP_SCRIPT_BODY));
	});
});
