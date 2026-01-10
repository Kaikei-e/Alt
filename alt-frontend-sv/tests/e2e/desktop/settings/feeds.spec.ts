import { expect, test } from "@playwright/test";
import { gotoDesktopRoute } from "../../helpers/navigation";
import {
	CONNECT_RSS_PATHS,
	RSS_FEED_DELETE_RESPONSE,
	RSS_FEED_LINKS_EMPTY_RESPONSE,
	RSS_FEED_LINKS_LIST_RESPONSE,
	RSS_FEED_REGISTER_RESPONSE,
} from "../../fixtures/mockData";
import { fulfillJson } from "../../utils/mockHelpers";

test.describe("desktop settings feeds - manage feed links", () => {
	test.beforeEach(async ({ page }) => {
		// Mock the RSS service endpoints
		await page.route(CONNECT_RSS_PATHS.listRSSFeedLinks, (route) =>
			fulfillJson(route, RSS_FEED_LINKS_LIST_RESPONSE),
		);
		await page.route(CONNECT_RSS_PATHS.registerRSSFeed, (route) =>
			fulfillJson(route, RSS_FEED_REGISTER_RESPONSE),
		);
		await page.route(CONNECT_RSS_PATHS.deleteRSSFeedLink, (route) =>
			fulfillJson(route, RSS_FEED_DELETE_RESPONSE),
		);
	});

	test("renders page with title and description", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		await expect(
			page.getByRole("heading", { name: "Manage Feed Links" }),
		).toBeVisible();
		await expect(
			page.getByText("Add, edit, or remove RSS feed sources"),
		).toBeVisible();
	});

	test("displays registered feed links after refresh", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Click refresh to trigger client-side fetch (SSR can't be mocked by page.route)
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		// Wait for feeds to load
		await expect(page.getByText("Registered Feeds")).toBeVisible();
		await expect(page.getByText("3")).toBeVisible({ timeout: 10000 }); // Badge showing count

		// Check that all feed URLs are displayed
		await expect(page.getByText("https://example.com/feed.xml")).toBeVisible();
		await expect(page.getByText("https://blog.example.org/rss")).toBeVisible();
		await expect(page.getByText("https://news.site.com/atom.xml")).toBeVisible();
	});

	test("displays empty state when no feeds registered", async ({ page }) => {
		// Override with empty response
		await page.route(CONNECT_RSS_PATHS.listRSSFeedLinks, (route) =>
			fulfillJson(route, RSS_FEED_LINKS_EMPTY_RESPONSE),
		);

		await gotoDesktopRoute(page, "settings/feeds");

		// Click refresh to trigger client-side fetch
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		await expect(
			page.getByText("No feeds registered yet. Add your first feed using the form."),
		).toBeVisible();
	});

	test("shows add feed form with input and button", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		await expect(page.getByText("Add New Feed")).toBeVisible();
		await expect(
			page.getByPlaceholder("https://example.com/feed.xml"),
		).toBeVisible();
		await expect(page.getByRole("button", { name: "Add Feed" })).toBeVisible();
	});

	test("validates empty URL on form submission", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Click Add Feed without entering URL
		await page.getByRole("button", { name: "Add Feed" }).click();

		// Should show validation error
		await expect(page.getByText("Please enter the RSS URL.")).toBeVisible();
	});

	test("validates invalid URL format", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Enter invalid URL
		await page
			.getByPlaceholder("https://example.com/feed.xml")
			.fill("not-a-valid-url");
		await page.getByRole("button", { name: "Add Feed" }).click();

		// Should show validation error (exact message depends on schema)
		const errorMessage = page.locator("text=/invalid|url/i");
		await expect(errorMessage).toBeVisible();
	});

	test("validates URL that doesn't look like a feed", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Enter URL that's not a feed
		await page
			.getByPlaceholder("https://example.com/feed.xml")
			.fill("https://example.com/some/random/page");
		await page.getByRole("button", { name: "Add Feed" }).click();

		// Should show validation error - exact message from feedUrlSchema
		await expect(
			page.getByText("URL does not appear to be a valid RSS or Atom feed"),
		).toBeVisible();
	});

	test("successfully registers a new feed", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Enter valid feed URL
		await page
			.getByPlaceholder("https://example.com/feed.xml")
			.fill("https://newsite.com/feed.xml");
		await page.getByRole("button", { name: "Add Feed" }).click();

		// Should show success message
		await expect(page.getByText("Feed registered successfully.")).toBeVisible();

		// Input should be cleared
		await expect(
			page.getByPlaceholder("https://example.com/feed.xml"),
		).toHaveValue("");
	});

	test("shows error message when registration fails", async ({ page }) => {
		// Mock the register API to fail
		await page.route(CONNECT_RSS_PATHS.registerRSSFeed, (route) =>
			fulfillJson(route, { code: "already_exists", message: "Feed already exists" }, 400),
		);

		await gotoDesktopRoute(page, "settings/feeds");

		// Enter valid feed URL
		await page
			.getByPlaceholder("https://example.com/feed.xml")
			.fill("https://duplicate.com/feed.xml");
		await page.getByRole("button", { name: "Add Feed" }).click();

		// Should show error message
		await expect(page.getByText(/Error/)).toBeVisible();
	});

	test("opens delete confirmation dialog", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Click refresh to load feeds
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		// Wait for feeds to load
		await expect(page.getByText("https://example.com/feed.xml")).toBeVisible({ timeout: 10000 });

		// Click delete button on first feed
		const deleteButtons = page.getByRole("button", { name: "Delete feed link" });
		await deleteButtons.first().click();

		// Dialog should appear
		await expect(page.getByText("Delete Feed Link?")).toBeVisible();
		await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();
		await expect(page.getByRole("button", { name: "Delete", exact: true })).toBeVisible();
	});

	test("cancels delete operation", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Click refresh to load feeds
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		// Wait for feeds to load and click delete
		await expect(page.getByText("https://example.com/feed.xml")).toBeVisible({ timeout: 10000 });
		const deleteButtons = page.getByRole("button", { name: "Delete feed link" });
		await deleteButtons.first().click();

		// Cancel the dialog
		await page.getByRole("button", { name: "Cancel" }).click();

		// Dialog should close
		await expect(page.getByText("Delete Feed Link?")).not.toBeVisible();

		// Feed should still be visible
		await expect(page.getByText("https://example.com/feed.xml")).toBeVisible();
	});

	test("successfully deletes a feed link", async ({ page }) => {
		await gotoDesktopRoute(page, "settings/feeds");

		// Click refresh to load feeds
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		// Wait for feeds to load
		await expect(page.getByText("https://example.com/feed.xml")).toBeVisible({ timeout: 10000 });

		// Click delete button
		const deleteButtons = page.getByRole("button", { name: "Delete feed link" });
		await deleteButtons.first().click();

		// Confirm delete
		await page.getByRole("button", { name: "Delete", exact: true }).click();

		// Should show success message
		await expect(page.getByText("Feed link deleted.")).toBeVisible();
	});

	test("refresh button reloads feed list", async ({ page }) => {
		let requestCount = 0;
		await page.route(CONNECT_RSS_PATHS.listRSSFeedLinks, async (route) => {
			requestCount++;
			await fulfillJson(route, RSS_FEED_LINKS_LIST_RESPONSE);
		});

		await gotoDesktopRoute(page, "settings/feeds");

		// Wait for page to be ready
		await expect(page.getByText("Registered Feeds")).toBeVisible();

		// Click refresh button
		await page.getByRole("button", { name: "Refresh feed list" }).click();

		// Wait for the request to complete
		await page.waitForTimeout(500);

		// Should have made at least one request
		expect(requestCount).toBeGreaterThanOrEqual(1);
	});
});
