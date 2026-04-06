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
});
