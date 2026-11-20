import { expect, test } from "../fixtures/pomFixtures";

/**
 * Regression gate for the SSR preload-warning fix.
 *
 * SvelteKit emits CSS chunks both through the `Link:` HTTP header (as
 * `rel="preload"; as="style"`) and through HTML `<head>` (as
 * `rel="stylesheet"`). For chunks that overlap, Chrome flags the preload as
 * "preloaded using link preload but not used within a few seconds from the
 * window's load event." `kit.inlineStyleThreshold` was raised in
 * vite.config.ts to inline route-specific small CSS chunks and erase the
 * duplicate, so most routes should land at zero warnings post-fix. One stray
 * warning (root layout's large CSS) is tolerated until upstream SvelteKit
 * Issue #8549 lands the `modulepreload: 'tag' | 'header'` switch — when that
 * ships, tighten the threshold to 0.
 *
 * Reference:
 *   https://github.com/sveltejs/kit/issues/8549
 *   https://www.debugbear.com/blog/rel-preload-problems
 */

const PRELOAD_WARNING_FRAGMENT = "preloaded using link preload but not used";

// Per-route warning ceiling. The root layout CSS chunk is large (~85 KB) and
// stays external + preloaded by SvelteKit's default behavior, so one preload
// warning may legitimately persist per route until upstream #8549 lands.
const MAX_WARNINGS_PER_ROUTE = 1;

const ROUTES = ["/", "/feeds", "/home"] as const;

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
			// biome-ignore lint/nursery/noPlaywrightNetworkidle: regression gate monitors arbitrary routes' subresource preloads with no single target element
			await page.waitForLoadState("networkidle");
			// biome-ignore lint/nursery/noPlaywrightWaitForTimeout: Chromium emits preload-unused console warnings via an internal ~3-5s post-load browser heuristic with no DOM/network signal
			await page.waitForTimeout(5000);

			expect(
				warnings.length,
				`Unexpected preload warnings on ${route}:\n${warnings.join("\n")}`,
			).toBeLessThanOrEqual(MAX_WARNINGS_PER_ROUTE);
		});
	}
});
