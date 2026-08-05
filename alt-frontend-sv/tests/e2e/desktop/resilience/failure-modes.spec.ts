import { expect, test } from "../../fixtures/pomFixtures";
import { DesktopKnowledgeHomePage } from "../../pages/desktop/DesktopKnowledgeHomePage";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { buildGetKnowledgeHomeResponse } from "../../fixtures/factories/knowledgeHomeFactory";

/**
 * What the app does when the backend does not cooperate.
 *
 * The suite was almost entirely happy-path: an endpoint either returned its
 * fixture or, in one or two specs, a 500. The failures that actually strand a
 * user — the connection dropping mid-page, a session expiring while the tab is
 * open — went unexercised, and they are the ones where a silent `catch` leaves
 * a permanent spinner.
 *
 * Deliberately narrow: E2E covers the failure paths a user can get stuck in.
 * Per-field validation and error-message wording belong in unit tests.
 */

const KH_GET =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome";
const KH_TRACK_SEEN =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeItemsSeen";
const KH_RECALL =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/GetRecallRail";
const KH_STREAM =
	"**/api/v2/alt.knowledge_home.v1.KnowledgeHomeService/StreamKnowledgeHomeUpdates";
const LIST_SUBS = "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";

test.describe("connection loss", () => {
	test("feeds surfaces an error instead of spinning forever", async ({
		page,
		desktopFeedsPage,
	}) => {
		// A dropped connection, not an HTTP error — these take different paths
		// through the Connect client, and only the HTTP one was covered.
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			route.abort("connectionfailed"),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			route.abort("connectionfailed"),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await desktopFeedsPage.goto();

		await expect(desktopFeedsPage.errorMessage).toBeVisible({
			timeout: 15_000,
		});
		// The stuck-forever symptom is the spinner outliving the failure.
		await expect(desktopFeedsPage.loadingSpinner).toBeHidden();
	});

	test("recovers when the connection comes back", async ({
		page,
		desktopFeedsPage,
	}) => {
		// One failure must not poison the page for the rest of the session.
		let attempt = 0;
		const respond = (route: import("@playwright/test").Route) => {
			attempt += 1;
			return attempt === 1
				? route.abort("connectionfailed")
				: fulfillJson(route, CONNECT_FEEDS_RESPONSE);
		};
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, respond);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, respond);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await desktopFeedsPage.goto();
		await expect(desktopFeedsPage.errorMessage).toBeVisible({
			timeout: 15_000,
		});

		// A reload is the recovery a user actually performs.
		await desktopFeedsPage.goto();

		await expect(desktopFeedsPage.getFeedCardByTitle("AI Trends")).toBeVisible({
			timeout: 15_000,
		});
		await expect(desktopFeedsPage.errorMessage).toBeHidden();
	});
});

test.describe("session expiry", () => {
	/**
	 * The matching positive case — Code.Unauthenticated must call
	 * `goto("/login")` — lives in `src/lib/hooks/useKnowledgeHome.test.ts`, not
	 * here, and deliberately so.
	 *
	 * Driving it end-to-end by fulfilling GetKnowledgeHome with a Connect 401
	 * does NOT reach that branch: the browser logs the 401, the store never
	 * redirects, and the page settles on /home. The same rejection built as a
	 * real `ConnectError(…, Code.Unauthenticated)` does redirect at the unit
	 * level. So either the fulfilled response is not wire-faithful enough for
	 * connect-web to classify, or `err instanceof ConnectError` does not hold for
	 * the bundled client — worth resolving, but not something an assertion here
	 * can decide. Asserting the redirect from E2E would only encode whichever
	 * answer happens to be true today.
	 */
	test("a non-auth backend error keeps the user on the page", async ({
		page,
	}) => {
		// The mirror image: a 500 must NOT be mistaken for an expired session and
		// bounce someone out of the app they are still signed into.
		await Promise.all([
			page.route(KH_GET, (route) =>
				fulfillConnectError(route, "boom", "internal", 500),
			),
			page.route(KH_TRACK_SEEN, (route) => fulfillJson(route, {})),
			page.route(KH_RECALL, (route) => fulfillJson(route, { candidates: [] })),
			page.route(LIST_SUBS, (route) => fulfillJson(route, { sources: [] })),
			page.route(KH_STREAM, (route) => route.abort()),
		]);

		await new DesktopKnowledgeHomePage(page).goto();

		// Poll rather than assert once: the bug this guards against is a delayed
		// navigation, which a single immediate check would miss.
		await expect
			.poll(() => page.url(), { timeout: 5_000, intervals: [500, 1000, 2000] })
			.not.toMatch(/\/login/);
	});
});

test.describe("slow backend", () => {
	test("shows a loading state while the first page is in flight", async ({
		page,
		desktopFeedsPage,
	}) => {
		// Without a visible pending state a slow network looks like a dead page.
		let release: (() => void) | undefined;
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});

		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		for (const path of [
			CONNECT_RPC_PATHS.getAllFeeds,
			CONNECT_RPC_PATHS.getUnreadFeeds,
		]) {
			await page.route(path, async (route) => {
				await held;
				await fulfillJson(route, CONNECT_FEEDS_RESPONSE);
			});
		}

		await desktopFeedsPage.goto();
		await expect(desktopFeedsPage.loadingSpinner).toBeVisible();

		release?.();

		await expect(desktopFeedsPage.getFeedCardByTitle("AI Trends")).toBeVisible({
			timeout: 15_000,
		});
		await expect(desktopFeedsPage.loadingSpinner).toBeHidden();
	});
});

test.describe("degraded knowledge home", () => {
	test("still renders items when the recall rail fails", async ({ page }) => {
		// One failing side panel must not take the main stream down with it.
		await Promise.all([
			page.route(KH_GET, (route) =>
				fulfillJson(route, buildGetKnowledgeHomeResponse()),
			),
			page.route(KH_TRACK_SEEN, (route) => fulfillJson(route, {})),
			page.route(KH_RECALL, (route) =>
				fulfillConnectError(route, "recall down", "unavailable", 503),
			),
			page.route(LIST_SUBS, (route) => fulfillJson(route, { sources: [] })),
			page.route(KH_STREAM, (route) => route.abort()),
		]);

		const homePage = new DesktopKnowledgeHomePage(page);
		await homePage.goto();

		await expect(page).not.toHaveURL(/\/login/);
		await expect(
			page.getByText("Rust Async Runtime Deep Dive").first(),
		).toBeVisible({ timeout: 15_000 });
	});
});
