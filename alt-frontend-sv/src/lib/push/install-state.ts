/**
 * Is this document running as an installed Home Screen / standalone app?
 *
 * On iOS this question decides whether Web Push can work at all — a plain
 * Safari tab has no PushManager — so the answer drives what the notification
 * settings page is allowed to offer.
 *
 * Three signals are consulted rather than the one the spec suggests:
 *
 *  - `navigator.standalone` is non-standard and iOS-only, and it is the only
 *    one iOS has always answered correctly.
 *  - `display-mode: standalone` is the standard query, correct on Android and
 *    desktop Chrome.
 *  - `display-mode: fullscreen` is included because installed iOS web apps
 *    report *fullscreen* rather than *standalone* (WebKit bug 264218). Relying
 *    on the standard query alone tells every installed iOS user to install.
 */

interface StandaloneNavigator extends Navigator {
	/** Non-standard, iOS only. */
	standalone?: boolean;
}

const INSTALLED_DISPLAY_MODES = ["standalone", "fullscreen"] as const;

export function isInstalled(win: Window | undefined): boolean {
	if (!win) return false;

	const nav = win.navigator as StandaloneNavigator | undefined;
	if (nav?.standalone === true) return true;

	if (typeof win.matchMedia !== "function") return false;

	return INSTALLED_DISPLAY_MODES.some(
		(mode) => win.matchMedia(`(display-mode: ${mode})`).matches,
	);
}
