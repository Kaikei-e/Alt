import { expect, test } from "@playwright/test";
import { fulfillJson } from "../../utils/mockHelpers";

const FETCH_CONTENT =
	"**/api/v2/alt.articles.v2.ArticleService/FetchArticleContent";

test.describe("Desktop Article Detail", () => {
	test("renders article content and action buttons", async ({ page }) => {
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: "<p>This is the article body content.</p>",
				article_id: "art-123",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: "<p>This is the article body content.</p>",
				article_id: "art-123",
			}),
		);

		await page.goto(
			"./articles/art-123?url=https%3A%2F%2Fexample.com%2Ftest&title=Test%20Article",
		);
		await page.waitForLoadState("domcontentloaded");

		// Article content should be rendered
		await expect(
			page.getByText("This is the article body content."),
		).toBeVisible({ timeout: 10000 });

		// Action buttons should be present
		await expect(page.getByText("Open original")).toBeVisible();
	});

	test("shows back navigation to home", async ({ page }) => {
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: "<p>Content here.</p>",
				article_id: "art-456",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: "<p>Content here.</p>",
				article_id: "art-456",
			}),
		);

		await page.goto(
			"./articles/art-456?url=https%3A%2F%2Fexample.com&title=Nav%20Test",
		);
		await page.waitForLoadState("domcontentloaded");

		// Back to Home button should be present
		await expect(
			page.getByRole("button", { name: /back to home/i }),
		).toBeVisible({ timeout: 10000 });
	});

	test("renders editorial rail on desktop", async ({ page }) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: `${"<p>Article body paragraph one. ".repeat(30)}</p>`,
				article_id: "art-rail",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: `${"<p>Article body paragraph one. ".repeat(30)}</p>`,
				article_id: "art-rail",
			}),
		);

		await page.goto(
			"./articles/art-rail?url=https%3A%2F%2Fexample.com%2Fdeep%2Fpath&title=Rail%20Article",
		);
		await page.waitForLoadState("domcontentloaded");

		const rail = page.getByTestId("article-rail");
		await expect(rail).toBeVisible({ timeout: 10000 });

		// Rail carries source, reading-time metadata and primary actions
		await expect(rail.getByText(/source/i)).toBeVisible();
		await expect(rail.getByText(/reading time/i)).toBeVisible();
		await expect(rail.getByText(/example\.com/i)).toBeVisible();
		await expect(
			rail.getByRole("link", { name: /open original/i }),
		).toBeVisible();
		await expect(rail.getByRole("button", { name: /summariz/i })).toBeVisible();
	});

	test("article body uses centered reading measure, not pinned-left", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 1440, height: 900 });
		const longBody = `<p>${"word ".repeat(400)}</p>`;
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: longBody,
				article_id: "art-measure",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: longBody,
				article_id: "art-measure",
			}),
		);

		await page.goto(
			"./articles/art-measure?url=https%3A%2F%2Fexample.com&title=Measure",
		);
		await page.waitForLoadState("domcontentloaded");

		const content = page.locator(".article-content").first();
		await expect(content).toBeVisible({ timeout: 10000 });

		const box = await content.boundingBox();
		expect(box).not.toBeNull();
		if (!box) return;

		// Body measure stays within readable band (≈55–80ch at current font)
		expect(box.width).toBeGreaterThanOrEqual(480);
		expect(box.width).toBeLessThanOrEqual(760);

		// Body must not be pinned to the far left of the main area:
		// its left edge should be meaningfully offset from the sidebar edge.
		// At 1440px viewport with ~240px sidebar, anything under 280px means pinned-left.
		expect(box.x).toBeGreaterThan(280);
	});

	test("rail is hidden on mobile viewport", async ({ page }) => {
		await page.setViewportSize({ width: 390, height: 844 });
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: "<p>Mobile body.</p>",
				article_id: "art-mobile",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: "<p>Mobile body.</p>",
				article_id: "art-mobile",
			}),
		);

		await page.goto(
			"./articles/art-mobile?url=https%3A%2F%2Fexample.com&title=Mobile",
		);
		await page.waitForLoadState("domcontentloaded");

		await expect(page.getByText("Mobile body.")).toBeVisible({
			timeout: 10000,
		});

		// Rail never renders on narrow viewports — ADR-000715 mobile composition preserved
		await expect(page.getByTestId("article-rail")).toHaveCount(0);

		// Top sticky action-bar actions remain reachable on mobile
		await expect(
			page.getByRole("button", { name: /back to home/i }),
		).toBeVisible();
		await expect(
			page.getByRole("link", { name: /open original/i }),
		).toBeVisible();
	});
});
