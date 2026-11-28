import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";

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

const LIST_ARTICLE_ANNOTATIONS =
	"**/api/v2/alt.reading.v1.ReadingService/ListArticleAnnotations";

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
	test("a non-auth backend error keeps the user on the page", async ({
		page,
	}) => {
		// A 500 must NOT be mistaken for an expired session and bounce someone out
		// of the app they are still signed into.
		await Promise.all([
			page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
				fulfillConnectError(route, "boom", "internal", 500),
			),
			page.route(LIST_ARTICLE_ANNOTATIONS, (route) =>
				fulfillJson(route, { annotations: [] }),
			),
		]);

		await page.goto("./today");

		// Poll rather than assert once: the bug this guards against is a delayed
		// navigation, which a single immediate check would miss.
		await expect
			.poll(() => page.url(), { timeout: 5_000, intervals: [500, 1000, 2000] })
			.not.toMatch(/\/login/);
		await expect(page.getByTestId("today-error")).toBeVisible({
			timeout: 10_000,
		});
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

test.describe("degraded today surface", () => {
	test("still renders queued items when recall annotations fail", async ({
		page,
	}) => {
		// One failing secondary panel must not take the main reading queue down.
		const now = Date.now();
		await Promise.all([
			page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
				fulfillJson(route, {
					items: [
						{
							articleId: "01938e82-7c00-7a7b-9b10-000000000001",
							title: "Rust Async Runtime Deep Dive",
							url: "https://example.com/rust-async",
							sourceName: "Rust Blog",
							imageUrl: "",
							queuedAt: new Date(now).toISOString(),
							expiresAt: new Date(now + 48 * 3600_000).toISOString(),
						},
					],
					cap: 7,
					readCountSince: 0,
				}),
			),
			page.route(LIST_ARTICLE_ANNOTATIONS, (route) =>
				fulfillConnectError(route, "recall down", "unavailable", 503),
			),
		]);

		await page.goto("./today");

		await expect(page).not.toHaveURL(/\/login/);
		await expect(
			page.getByText("Rust Async Runtime Deep Dive").first(),
		).toBeVisible({ timeout: 15_000 });
	});
});
