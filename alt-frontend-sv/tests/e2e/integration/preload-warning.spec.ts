import { test, expect } from "@playwright/test";

/**
 * Regression gate for the SSR preload-warning fix.
 *
 * SvelteKit emits CSS chunks both through the `Link:` HTTP header (as
 * `rel="preload"; as="style"`) and through HTML `<head>` (as
 * `rel="stylesheet"`). For chunks that overlap, Chrome flags the preload as
 * "preloaded using link preload but not used within a few seconds from the
 * window's load event." `kit.inlineStyleThreshold` was raised in
 * svelte.config.js to inline route-specific small CSS chunks and erase the
 * duplicate, so most routes should land at zero warnings post-fix. One stray
 * warning (root layout's large CSS) is tolerated until upstream SvelteKit
 * Issue #8549 lands the `modulepreload: 'tag' | 'header'` switch — when that
 * ships, tighten the threshold to 0.
 *
 * Reference:
 *   https://github.com/sveltejs/kit/issues/8549
 *   https://www.debugbear.com/blog/rel-preload-problems
 */

const PRELOAD_WARNING_FRAGMENT =
	"preloaded using link preload but not used";

// Per-route warning ceiling. The root layout CSS chunk is large (~85 KB) and
// stays external + preloaded by SvelteKit's default behavior, so one preload
// warning may legitimately persist per route until upstream #8549 lands.
const MAX_WARNINGS_PER_ROUTE = 1;

const ROUTES = ["/", "/loop", "/home"] as const;

test.describe("Preload warning regression gate", () => {
	for (const route of ROUTES) {
		test(`${route} emits at most ${MAX_WARNINGS_PER_ROUTE} preload-not-used warnings`, async ({
			page,
		}) => {
			const warnings: string[] = [];
			page.on("console", (msg) => {
				if (
					msg.type() === "warning" &&
					msg.text().includes(PRELOAD_WARNING_FRAGMENT)
				) {
					warnings.push(msg.text());
				}
			});

			await page.goto(`.${route}`);
			// Wait for network idle + Chrome's "few-second" window after load
			// so any deferred preload directive surfaces as a warning if it is
			// going to. Five seconds matches the Chromium heuristic timer.
			await page.waitForLoadState("networkidle");
			await page.waitForTimeout(5000);

			expect(
				warnings.length,
				`Unexpected preload warnings on ${route}:\n${warnings.join("\n")}`,
			).toBeLessThanOrEqual(MAX_WARNINGS_PER_ROUTE);
		});
	}
});
