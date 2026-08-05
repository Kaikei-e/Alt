/**
 * Base Playwright test with the hygiene every Alt E2E spec needs.
 *
 * Two auto-fixtures run for every test that derives from this base:
 *
 *  1. `blockExternalOrigins` — keeps the browser off the public internet.
 *     `app.html` links a Google Fonts stylesheet in `<head>`. It is
 *     render-blocking, so on a machine without egress every single
 *     `page.goto()` sat on it for ~13s before the socket reset — the dominant
 *     cost of the whole suite. Even where egress exists, a third-party font CDN
 *     is exactly the "don't test what you don't control" dependency Playwright's
 *     best-practice guide says to stub out: it adds latency and a flake source
 *     to tests that are not about fonts.
 *
 *  2. `pageErrors` — fails a test whose page threw an uncaught exception.
 *     A spec that only asserts on visible text happily passes while the console
 *     fills with hydration or store errors; this turns those into failures
 *     without every spec having to remember to wire up a listener.
 *
 * Specs opt out of (2) per-file with:
 *   test.use({ allowedPageErrors: [/ResizeObserver loop/] });
 */
import { test as base, expect } from "@playwright/test";

/**
 * Hosts served by the E2E stack itself (SvelteKit preview + mock Kratos /
 * AuthHub / Backend). Everything else is third-party as far as a test is
 * concerned.
 */
const LOCAL_HOSTNAMES = new Set(["127.0.0.1", "localhost", "::1", "[::1]"]);

/** Font CDNs are stubbed rather than aborted so `<link rel=stylesheet>` resolves cleanly. */
const FONT_HOSTNAMES = new Set(["fonts.googleapis.com", "fonts.gstatic.com"]);

function isLocalUrl(url: URL): boolean {
	return LOCAL_HOSTNAMES.has(url.hostname);
}

export type BaseFixtures = {
	/** Auto-fixture; nothing to consume. Installs the third-party request guard. */
	blockExternalOrigins: void;
	/**
	 * Uncaught page exceptions observed during the test, in order. Read it to
	 * assert on an error the app is *supposed* to raise.
	 */
	pageErrors: Error[];
	/**
	 * Patterns for uncaught exceptions that must not fail the test. Override per
	 * file with `test.use({ allowedPageErrors: [...] })`.
	 */
	allowedPageErrors: RegExp[];
};

export const test = base.extend<BaseFixtures>({
	allowedPageErrors: [[], { option: true }],

	blockExternalOrigins: [
		async ({ context }, use) => {
			await context.route(
				(url) => !isLocalUrl(url),
				async (route) => {
					const url = new URL(route.request().url());

					if (FONT_HOSTNAMES.has(url.hostname)) {
						// An empty stylesheet: the document falls back to the system
						// font stack, which is what a failed font fetch produced anyway.
						await route.fulfill({
							status: 200,
							contentType: "text/css",
							body: "",
						});
						return;
					}

					await route.abort("blockedbyclient");
				},
			);

			await use(undefined);
		},
		{ auto: true },
	],

	pageErrors: [
		async ({ page, allowedPageErrors }, use) => {
			const errors: Error[] = [];
			page.on("pageerror", (error) => errors.push(error));

			await use(errors);

			const unexpected = errors.filter(
				(error) =>
					!allowedPageErrors.some((pattern) => pattern.test(error.message)),
			);

			expect(
				unexpected.map((error) => error.message),
				"page threw uncaught exception(s)",
			).toEqual([]);
		},
		{ auto: true },
	],
});

export { expect };
