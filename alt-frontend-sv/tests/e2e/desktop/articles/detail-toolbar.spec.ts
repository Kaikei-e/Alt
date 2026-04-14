import { expect, test } from "@playwright/test";
import { fulfillJson } from "../../utils/mockHelpers";

const FETCH_CONTENT =
	"**/api/v2/alt.articles.v2.ArticleService/FetchArticleContent";

test.describe("Desktop Article Detail — toolbar layout", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(FETCH_CONTENT, (route) =>
			fulfillJson(route, {
				content: "<p>body</p>",
				article_id: "art-tb-1",
			}),
		);
		await page.route("**/api/client/articles/content**", (route) =>
			fulfillJson(route, {
				content: "<p>body</p>",
				article_id: "art-tb-1",
			}),
		);

		await page.goto(
			"./articles/art-tb-1?url=https%3A%2F%2Fexample.com%2Ftb&title=Toolbar%20Test",
		);
		await page.waitForLoadState("domcontentloaded");
		await expect(page.getByText("body")).toBeVisible({ timeout: 10000 });
	});

	test("action labels are revealed on desktop viewport", async ({ page }) => {
		const summarizeBtn = page.getByTestId("summarize-button");
		await expect(summarizeBtn).toBeVisible();

		// toBeVisible() guards against display:none / visibility:hidden /
		// zero-size — which is the real invariant we care about. Computed
		// `display` cannot be asserted directly because flex-item children are
		// "blockified" by the spec, so a span inside inline-flex reports "block"
		// even when CSS sets `display: inline`.
		await expect(summarizeBtn.locator(".action-label")).toBeVisible();
		await expect(
			page.getByTestId("fetch-button").locator(".action-label"),
		).toBeVisible();
	});

	test("summarize button expands beyond icon-only width on desktop", async ({
		page,
	}) => {
		const summarizeBtn = page.getByTestId("summarize-button");
		await expect(summarizeBtn).toBeVisible();

		const box = await summarizeBtn.boundingBox();
		expect(box).not.toBeNull();
		// size="icon" yields a 36px square. On desktop the button must grow to
		// fit "Summarize" + icon; anything ≤ 40px means the label is clipping
		// the icon (the regression we're guarding against).
		expect(box!.width).toBeGreaterThan(60);
	});

	test("open-original link and summarize button share the same height", async ({
		page,
	}) => {
		const summarizeBtn = page.getByTestId("summarize-button");
		const openOriginal = page.getByRole("link", { name: /open original/i });

		await expect(summarizeBtn).toBeVisible();
		await expect(openOriginal).toBeVisible();

		const btnBox = await summarizeBtn.boundingBox();
		const linkBox = await openOriginal.boundingBox();
		expect(btnBox).not.toBeNull();
		expect(linkBox).not.toBeNull();
		// Visual parity check: toolbar controls must align. Allow 2px of AA/
		// sub-pixel rounding slack.
		expect(Math.abs(btnBox!.height - linkBox!.height)).toBeLessThanOrEqual(2);
	});
});
