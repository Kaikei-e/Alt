import { expect, test } from "../../fixtures/pomFixtures";

/**
 * The app must be installable to the iOS Home Screen, because Web Push on
 * iOS is only delivered to a Home Screen web app — a plain Safari tab has no
 * PushManager. So installability is not cosmetic here; it is the gate every
 * later notification test sits behind.
 *
 * What is asserted, and why each field is load-bearing:
 *
 *  - the manifest is reachable *without a session*. The install prompt is
 *    evaluated before the user signs in, and `route-guard.ts` 303s every
 *    unlisted path to login. A manifest that redirects is a manifest the
 *    browser never parses.
 *  - `display: "standalone"`. WebKit honours only `standalone` and `browser`;
 *    `fullscreen` and `minimal-ui` are ignored, and `display_override` is not
 *    parsed at all. Anything else silently degrades to a browser tab, which
 *    takes PushManager away again.
 *  - a 192 and a 512 icon, plus a separate `maskable` one. A maskable icon is
 *    cropped to a circle on Android; reusing the `any` icon there clips the
 *    artwork rather than failing loudly.
 *  - `id` and `scope`. `scope` became load-bearing on iOS once out-of-scope
 *    links started opening in an in-app browser view *inside* the web app
 *    rather than handing off to Safari.
 */

const REQUIRED_ICON_SIZES = ["192x192", "512x512"];

type ManifestIcon = {
	src: string;
	sizes: string;
	type?: string;
	purpose?: string;
};

type WebManifest = {
	id?: string;
	name?: string;
	short_name?: string;
	start_url?: string;
	scope?: string;
	display?: string;
	theme_color?: string;
	background_color?: string;
	icons?: ManifestIcon[];
};

test.describe("PWA installability", () => {
	test("serves a manifest that an unauthenticated browser can parse", async ({
		browser,
		baseURL,
	}) => {
		// A signed-out context on purpose: the install prompt is evaluated
		// before sign-in, so the manifest has to be a public route.
		const anonymous = await browser.newContext({
			storageState: { cookies: [], origins: [] },
		});

		try {
			const response = await anonymous.request.get(
				new URL("/manifest.webmanifest", baseURL).href,
				{ maxRedirects: 0 },
			);

			expect(
				response.status(),
				"manifest must be public — a 303 to /auth/login means it is missing from PUBLIC_ROUTES",
			).toBe(200);
			expect(response.headers()["content-type"]).toContain(
				"application/manifest+json",
			);

			const manifest = (await response.json()) as WebManifest;

			expect(manifest.name).toBeTruthy();
			expect(manifest.short_name).toBeTruthy();
			expect(manifest.id).toBeTruthy();
			expect(manifest.start_url).toBeTruthy();
			expect(manifest.scope).toBeTruthy();
			expect(manifest.theme_color).toBeTruthy();
			expect(manifest.background_color).toBeTruthy();

			expect(
				manifest.display,
				"WebKit honours only standalone|browser, and standalone is what keeps PushManager available",
			).toBe("standalone");

			const icons = manifest.icons ?? [];
			for (const size of REQUIRED_ICON_SIZES) {
				expect(
					icons.some(
						(icon) =>
							icon.sizes === size && (icon.purpose ?? "any").includes("any"),
					),
					`manifest must declare a ${size} icon with purpose "any"`,
				).toBe(true);
			}

			expect(
				icons.some((icon) => (icon.purpose ?? "").includes("maskable")),
				'manifest must declare a separate "maskable" icon, not reuse the "any" artwork',
			).toBe(true);

			// Every declared icon has to actually resolve, or the install prompt
			// is suppressed with no error anywhere in the app.
			for (const icon of icons) {
				const iconResponse = await anonymous.request.get(
					new URL(icon.src, baseURL).href,
					{ maxRedirects: 0 },
				);
				expect(iconResponse.status(), `icon ${icon.src} must resolve`).toBe(
					200,
				);
			}
		} finally {
			await anonymous.close();
		}
	});

	test("links the manifest and an apple-touch-icon from the document head", async ({
		page,
	}) => {
		await page.goto("/");
		await page.waitForLoadState("domcontentloaded");

		await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
			"href",
			/manifest\.webmanifest$/,
		);

		// iOS still takes its Home Screen icon from this tag rather than the
		// manifest, so a manifest-only icon set installs with a screenshot.
		await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveCount(1);
	});

	test("registers a service worker that reaches activated", async ({
		page,
		browserName,
	}) => {
		test.skip(
			browserName === "webkit",
			"Playwright's WebKit build does not drive service-worker registration reliably; Chromium is the signal here and CI runs mobile-chrome",
		);

		await page.goto("/");
		await page.waitForLoadState("domcontentloaded");

		// The worker exists to receive `push`. It deliberately has no `fetch`
		// handler — the app already carries an 8-layer stale-asset defense that
		// assumes the network is authoritative, and a caching worker would sit
		// in front of all of it.
		await expect
			.poll(
				async () =>
					await page.evaluate(async () => {
						const registration =
							await navigator.serviceWorker.getRegistration();
						return registration?.active?.state ?? null;
					}),
				{
					timeout: 15_000,
					message: "service worker never reached the activated state",
				},
			)
			.toBe("activated");
	});
});
