import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * Source-level gate for the parts of `app.html` that only fail in production.
 *
 * The deployed origin sits behind Cloudflare Access; the E2E stack has no such
 * edge. Every assertion in `tests/e2e/mobile/pwa/installability.spec.ts` runs
 * against a stack where nothing intercepts the manifest request — the one
 * condition under which the bug guarded here cannot reproduce. A constraint
 * that only the unreachable environment can check is a comment, so it is
 * checked on the text instead, the same way `service-worker.source.spec.ts`
 * pins the push-only worker.
 */

const appHtml = readFileSync(
	fileURLToPath(new URL("./app.html", import.meta.url)),
	"utf-8",
);

describe("app.html head behind an authenticating edge", () => {
	it("fetches the manifest with credentials", () => {
		// Chromium decides this fetch's credentials mode from the crossorigin
		// attribute string alone and never consults same-origin-ness:
		// ManifestManager::ManifestUseCredentials() tests the attribute for the
		// literal "use-credentials", and manifest_fetcher.cc maps that to
		// kInclude and everything else to kOmit. So an unadorned link sends no
		// cookie even to our own origin, Cloudflare Access answers it the way it
		// answers any anonymous request — a redirect to *.cloudflareaccess.com —
		// and the browser never parses a manifest. Android then offers a
		// bookmark shortcut instead of installing a WebAPK.
		//
		// (The WHATWG HTML text for `rel=manifest` routes through the generic
		// CORS settings attribute table, which would make a bare attribute mean
		// "same-origin" and send the cookie. Chromium does not implement that,
		// and Chromium is what installs the app.)
		//
		// The attribute widens nothing. The URL is same-origin and already
		// served anonymously as a static asset; this only attaches a cookie the
		// browser already holds for this origin, and Access still decides.
		// Same-origin requests never reach Fetch's CORS check, so no
		// Access-Control-Allow-Origin is needed and nginx is unchanged.
		const link = /<link[^>]*rel="manifest"[^>]*>/.exec(appHtml)?.[0];

		expect(link, "app.html must link a manifest").toBeTruthy();
		expect(
			link,
			"without crossorigin=use-credentials the manifest request reaches Cloudflare Access unauthenticated and is redirected to the login domain",
		).toMatch(/crossorigin=["']use-credentials["']/);
	});
});
