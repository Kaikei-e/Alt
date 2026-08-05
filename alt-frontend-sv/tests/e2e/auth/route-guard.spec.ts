import { expect, test } from "../fixtures/pomFixtures";

/**
 * `hooks.server.ts` decides, for every request, whether it may proceed, must be
 * bounced to the login flow, or must be answered with a JSON 401. Nothing
 * covered it: the existing auth specs drive the Kratos login/registration
 * forms, not the guard that wraps the rest of the app.
 *
 * The guard has three distinct branches and they do NOT behave alike:
 *
 *  - no session cookie at all   → `validateSession` returns a null session
 *    rather than throwing, so the request proceeds and the SPA shell is served.
 *    Only the API is closed; there is no server-side redirect.
 *  - a cookie Kratos rejects    → `ory.toSession` throws, and the guard 303s
 *    page requests to `/login?return_to=…` and answers API requests with 401.
 *  - a public route             → served either way.
 *
 * The asymmetry between the first two is easy to "tidy up" into a bug, so it is
 * pinned here.
 */

const REJECTED_SESSION_COOKIE = {
	name: "ory_kratos_session",
	value: "not-a-valid-session",
	domain: "127.0.0.1",
	path: "/",
	httpOnly: true,
	secure: false,
	sameSite: "Lax",
} as const;

const FEEDS_RPC = "/api/v2/alt.feeds.v2.FeedService/GetAllFeeds";

test.describe("route guard — no session cookie", () => {
	test("serves the app shell for a protected route", async ({ page }) => {
		// `ssr = false` on the (app) layout means this HTML carries no user data:
		// it is the shell, and the client redirects once it fails to load data.
		const response = await page.request.get("/feeds", { maxRedirects: 0 });

		expect(response.status()).toBe(200);
	});

	test("still refuses the API", async ({ page }) => {
		// This is what actually protects the data.
		const response = await page.request.post(FEEDS_RPC, {
			data: {},
			maxRedirects: 0,
		});

		expect(response.status()).toBe(401);
	});
});

test.describe("route guard — rejected session cookie", () => {
	test.beforeEach(async ({ context }) => {
		await context.addCookies([REJECTED_SESSION_COOKIE]);
	});

	test("redirects a protected page to login, preserving the target", async ({
		page,
	}) => {
		const response = await page.request.get("/recap/job-status", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(303);
		expect(response.headers().location).toBe(
			"/login?return_to=%2Frecap%2Fjob-status",
		);
	});

	test("sends the root path to the feeds landing target", async ({ page }) => {
		// `/` has no path worth returning to, so the guard substitutes /feeds —
		// as an absolute URL, unlike every other route.
		const response = await page.request.get("/", { maxRedirects: 0 });

		expect(response.status()).toBe(303);
		expect(response.headers().location).toBe(
			"/login?return_to=http%3A%2F%2F127.0.0.1%3A4174%2Ffeeds",
		);
	});

	test("answers an API route with a JSON 401 rather than a redirect", async ({
		page,
	}) => {
		// A fetch() from the client cannot follow an HTML login redirect in any
		// useful way; it needs a status it can branch on.
		const response = await page.request.post(FEEDS_RPC, {
			data: {},
			maxRedirects: 0,
		});

		expect(response.status()).toBe(401);
		expect(response.headers()["content-type"]).toContain("application/json");
		expect(await response.json()).toMatchObject({
			error: "Authentication required",
		});
	});

	test("closes stream endpoints with the same 401", async ({ page }) => {
		// Stream endpoints take a separate branch in the guard
		// (`isStreamEndpoint`) that adds no-buffering headers; it must still deny.
		const response = await page.request.get("/api/v1/sse/stream", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(401);
	});

	test("leaves public routes reachable", async ({ page }) => {
		// A user whose session just expired has to be able to reach the pages
		// that let them recover, or the redirect above is a trap.
		for (const path of ["/error", "/health"]) {
			const response = await page.request.get(path, { maxRedirects: 0 });
			expect(response.status(), `${path} should stay public`).toBe(200);
		}
	});
});

test.describe("route guard — unknown paths", () => {
	test("returns 404 for a path outside the route tree", async ({ page }) => {
		// The `[...path]` catch-all throws error(404); without it an unknown path
		// falls into the guard and looks like an auth problem instead.
		const response = await page.request.get("/no-such-page", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(404);
	});
});
