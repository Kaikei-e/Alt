/**
 * Tests for Home Screen / standalone detection.
 *
 * This is not the one-liner it looks like. `display-mode: standalone` is the
 * documented way to ask, and it is **wrong on iOS**: WebKit reports
 * `standalone` as false and `fullscreen` as true for an installed Home Screen
 * web app (webkit.org bug 264218, open since 2023). Since iOS is the only
 * platform where installation is *mandatory* for Web Push, getting this
 * backwards means showing "add this to your Home Screen" to people who already
 * did — on the exact platform the instructions exist for.
 *
 * `navigator.standalone` is the non-standard property that has always been
 * right on iOS, so it is checked first and the media queries are the fallback.
 */

import { describe, expect, it } from "vitest";

import { isInstalled } from "./install-state";

type MatchResult = { matches: boolean };

/** Minimal stand-in for the parts of `window` this module reads. */
function makeWindow(options: {
	standalone?: boolean | undefined;
	displayModes?: string[];
	withoutMatchMedia?: boolean;
}): Window {
	const modes = new Set(options.displayModes ?? []);

	const win = {
		navigator: { standalone: options.standalone },
		matchMedia: options.withoutMatchMedia
			? undefined
			: (query: string): MatchResult => ({
					matches: [...modes].some((mode) =>
						query.includes(`display-mode: ${mode}`),
					),
				}),
	};

	return win as unknown as Window;
}

describe("isInstalled", () => {
	it("trusts navigator.standalone on iOS even though the media query disagrees", () => {
		// The real iOS shape: installed, but `display-mode: standalone` is false.
		const win = makeWindow({ standalone: true, displayModes: ["fullscreen"] });

		expect(isInstalled(win)).toBe(true);
	});

	it("treats display-mode: fullscreen as installed", () => {
		// Same iOS shape with navigator.standalone missing — a third-party
		// browser on iOS, or a future WebKit that drops the legacy property.
		const win = makeWindow({ displayModes: ["fullscreen"] });

		expect(isInstalled(win)).toBe(true);
	});

	it("treats display-mode: standalone as installed", () => {
		// Android / desktop Chrome, where the standard query is accurate.
		const win = makeWindow({ displayModes: ["standalone"] });

		expect(isInstalled(win)).toBe(true);
	});

	it("reports not installed for an ordinary browser tab", () => {
		const win = makeWindow({ standalone: false, displayModes: ["browser"] });

		expect(isInstalled(win)).toBe(false);
	});

	it("reports not installed rather than throwing when matchMedia is absent", () => {
		const win = makeWindow({ withoutMatchMedia: true });

		expect(isInstalled(win)).toBe(false);
	});

	it("reports not installed when there is no window at all", () => {
		// Guards the SSR path: the settings page renders server-side first.
		expect(isInstalled(undefined)).toBe(false);
	});
});
