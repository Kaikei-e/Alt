import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
	CONNECT_FEEDS_RESPONSE,
} from "../../fixtures/mockData";

/**
 * Moving between sections via the sidebar is how a desktop user gets anywhere,
 * and nothing tested it — every desktop spec deep-links straight to its page
 * with `goto()`, so a broken sidebar href or a collapsed section that refuses to
 * open would not have failed a single test. `SidebarComponent` existed as a page
 * object with no spec behind it.
 *
 * These assert the navigation contract only (does the link go where it says,
 * does the destination render, is the current section announced) and leave each
 * destination's own behaviour to that page's spec.
 */
test.describe("desktop sidebar navigation", () => {
	test.beforeEach(async ({ page }) => {
		// Landing on /feeds first; the destinations are asserted by URL and
		// heading, so only the starting page needs data.
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
	});

	test("renders the primary sections", async ({ page, sidebar }) => {
		await page.goto("./feeds");

		await expect(sidebar.brandTitle).toBeVisible();
		await expect(sidebar.getLink("Knowledge Home Today")).toBeVisible();
		await expect(sidebar.getLink("Your Trail")).toBeVisible();
		await expect(sidebar.getLink("Dashboard")).toBeVisible();
	});

	test("marks the current page with aria-current", async ({
		page,
		sidebar,
	}) => {
		await page.goto("./feeds");

		// The active link is styled with colour and weight alone, which is not
		// perceivable to a screen reader; aria-current is the announceable part
		// and the only non-CSS handle a test has on "where am I".
		await expect(sidebar.getLink("Unread Feeds")).toHaveAttribute(
			"aria-current",
			"page",
		);
		await expect(sidebar.getLink("Your Trail")).not.toHaveAttribute(
			"aria-current",
			"page",
		);
	});

	test("navigates to a top-level section", async ({ page, sidebar }) => {
		await page.goto("./feeds");

		await sidebar.navigateTo("Dashboard");

		await expect(page).toHaveURL(/\/dashboard$/);
		await expect(sidebar.getLink("Dashboard")).toHaveAttribute(
			"aria-current",
			"page",
		);
	});

	test("navigates to a nested section link", async ({ page, sidebar }) => {
		await page.goto("./feeds");

		// "Read History" lives under the collapsible Feeds group, which is open on
		// /feeds because one of its children is the current page.
		await sidebar.navigateTo("Read History");

		await expect(page).toHaveURL(/\/feeds\/viewed$/);
		await expect(sidebar.getLink("Read History")).toHaveAttribute(
			"aria-current",
			"page",
		);
	});

	test("expands a collapsed section to reveal its links", async ({
		page,
		sidebar,
	}) => {
		await page.goto("./feeds");

		const jobStatus = sidebar.getLink("Job Status");
		await expect(jobStatus).toBeHidden();

		await sidebar.toggleSection("Recap");

		await expect(jobStatus).toBeVisible();
	});

	test("collapses an expanded section again", async ({ page, sidebar }) => {
		await page.goto("./feeds");

		await sidebar.toggleSection("Recap");
		await expect(sidebar.getLink("Job Status")).toBeVisible();

		await sidebar.toggleSection("Recap");
		await expect(sidebar.getLink("Job Status")).toBeHidden();
	});

	test("keeps the sidebar usable after navigating", async ({
		page,
		sidebar,
	}) => {
		// A client-side navigation that tears down the layout would strand the
		// user on whatever page they landed on.
		await page.goto("./feeds");

		await sidebar.navigateTo("Dashboard");
		await expect(page).toHaveURL(/\/dashboard$/);

		await sidebar.navigateTo("Your Trail");
		await expect(page).toHaveURL(/\/knowledge\/trail$/);

		await expect(sidebar.brandTitle).toBeVisible();
	});

	test("every sidebar link points somewhere in the app", async ({
		page,
		sidebar,
	}) => {
		await page.goto("./feeds");

		const hrefs = await sidebar
			.getAllLinks()
			.evaluateAll((nodes) => nodes.map((n) => n.getAttribute("href")));

		expect(hrefs.length).toBeGreaterThan(0);
		for (const href of hrefs) {
			// Relative, in-app targets only — an absolute or empty href here means
			// a nav entry was wired up wrong.
			expect(href, "sidebar href").toMatch(/^\/[\w\-/?=&]*$/);
		}
	});
});
