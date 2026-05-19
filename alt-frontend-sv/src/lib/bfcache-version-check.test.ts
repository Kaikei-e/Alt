/**
 * Tests for the BFCache / tab-visibility version-check installer.
 *
 * `version.pollInterval` (5min) ships in ADR-000898 but cannot react on
 * the same tick as a BFCache restore. `installBfcacheVersionCheck`
 * triggers `updated.check()` on `pageshow.persisted` and on
 * `document.visibilityState === "visible"`, closing the race where the
 * user returns to a stale tab and immediately navigates before the
 * polling interval fires.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";

import { installBfcacheVersionCheck } from "./bfcache-version-check";

interface Harness {
	window: Window;
	document: Document;
	check: ReturnType<typeof vi.fn>;
	cleanup: () => void;
}

function makeHarness(): Harness {
	const win = new EventTarget() as unknown as Window;
	const docTarget = new EventTarget();
	let visibility: DocumentVisibilityState = "visible";
	Object.defineProperty(docTarget, "visibilityState", {
		configurable: true,
		get: () => visibility,
	});
	Object.defineProperty(docTarget, "__setVisibility", {
		configurable: true,
		value: (v: DocumentVisibilityState) => {
			visibility = v;
		},
	});
	const doc = docTarget as unknown as Document & {
		__setVisibility(v: DocumentVisibilityState): void;
	};
	const check = vi.fn();
	const cleanup = installBfcacheVersionCheck({
		window: win,
		document: doc,
		check,
	});
	return { window: win, document: doc, check, cleanup };
}

function firePageShow(win: Window, persisted: boolean) {
	const ev = new Event("pageshow");
	Object.defineProperty(ev, "persisted", {
		value: persisted,
		configurable: true,
	});
	win.dispatchEvent(ev);
}

function fireVisibility(
	doc: Document & { __setVisibility(v: DocumentVisibilityState): void },
	state: DocumentVisibilityState,
) {
	doc.__setVisibility(state);
	doc.dispatchEvent(new Event("visibilitychange"));
}

describe("installBfcacheVersionCheck", () => {
	let harness: Harness;

	beforeEach(() => {
		harness = makeHarness();
	});

	it("invokes check() on pageshow with persisted=true (BFCache restore)", () => {
		firePageShow(harness.window, true);
		expect(harness.check).toHaveBeenCalledTimes(1);
	});

	it("does NOT invoke check() on pageshow with persisted=false (regular navigation)", () => {
		firePageShow(harness.window, false);
		expect(harness.check).not.toHaveBeenCalled();
	});

	it("invokes check() when visibility flips to visible", () => {
		fireVisibility(
			harness.document as Document & {
				__setVisibility(v: DocumentVisibilityState): void;
			},
			"visible",
		);
		expect(harness.check).toHaveBeenCalledTimes(1);
	});

	it("does NOT invoke check() when visibility flips to hidden", () => {
		fireVisibility(
			harness.document as Document & {
				__setVisibility(v: DocumentVisibilityState): void;
			},
			"hidden",
		);
		expect(harness.check).not.toHaveBeenCalled();
	});

	it("removes both listeners on cleanup", () => {
		harness.cleanup();
		firePageShow(harness.window, true);
		fireVisibility(
			harness.document as Document & {
				__setVisibility(v: DocumentVisibilityState): void;
			},
			"visible",
		);
		expect(harness.check).not.toHaveBeenCalled();
	});
});
