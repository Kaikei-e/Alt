import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { THEME_UNOPENED_ID } from "../../infra/data/themes";
import { fulfillJson } from "../../utils/mockHelpers";

const MOCK_BACKEND_ORIGIN = "http://127.0.0.1:4003";

test.describe("Mobile Theme Pages (/knowledge)", () => {
	test.describe.configure({ mode: "serial" });

	test.beforeAll(async ({ request }) => {
		await request.post(`${MOCK_BACKEND_ORIGIN}/__mock/themes/reset`);
	});

	test("lists both themes with status suffixes, opens unopened theme, opens reader overlay from marker, switches version, and clears new suffix", async ({
		page,
		request,
	}) => {
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		await gotoMobileRoute(page, "knowledge");

		await expect(
			page.getByRole("heading", { level: 1, name: "Themes" }),
		).toBeVisible();
		await expect(
			page.getByText("Pages built only from articles you have read."),
		).toBeVisible();

		const rows = page.getByTestId("theme-row");
		await expect(rows).toHaveCount(2);

		const unopenedRow = rows.nth(0);
		await expect(unopenedRow).toContainText("Compiler Toolchains");
		await expect(unopenedRow).toContainText("3 articles · updated");
		await expect(unopenedRow).toContainText("· new");

		const openedRow = rows.nth(1);
		await expect(openedRow).toContainText("Storage Engines");
		await expect(openedRow).toContainText("2 articles · updated");
		await expect(openedRow).not.toContainText("· new");
		await expect(openedRow).not.toContainText("updated since you read it");

		await expect(
			page.getByRole("link", { name: "Knowledge Trail" }),
		).toHaveAttribute("href", "/knowledge/trail");

		// Open the unopened theme and confirm RecordThemeOpened fires
		const recordPromise = page.waitForResponse(
			(res) =>
				res.url().includes("ReadingService/RecordThemeOpened") &&
				res.status() === 200,
		);
		await unopenedRow.getByRole("link").click();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe(`/knowledge/themes/${THEME_UNOPENED_ID}`);
		await recordPromise;

		await expect(
			page.getByRole("heading", { level: 1, name: "Compiler Toolchains" }),
		).toBeVisible();
		await expect(page.getByTestId("theme-meta")).toContainText(
			"Version 2 of 2 ·",
		);
		await expect(page.getByTestId("theme-meta")).toContainText("· 3 sources");

		await expect(page.getByTestId("theme-section-key-points")).toBeVisible();
		await expect(page.getByTestId("theme-section-timeline")).toBeVisible();
		await expect(page.getByTestId("theme-section-what-changed")).toBeVisible();
		await expect(
			page.getByTestId("theme-section-open-questions"),
		).toBeVisible();
		await expect(page.getByTestId("theme-section-sources")).toBeVisible();
		await expect(page.getByTestId("theme-section-versions")).toBeVisible();

		const openedCallsRes = await request.get(
			`${MOCK_BACKEND_ORIGIN}/__mock/themes/opened`,
		);
		const openedCallsJson = (await openedCallsRes.json()) as {
			calls: Array<{ themeId: string; version: number }>;
		};
		expect(openedCallsJson.calls).toContainEqual(
			expect.objectContaining({
				themeId: THEME_UNOPENED_ID,
				version: 2,
			}),
		);

		// Tap a source marker to open the reader overlay
		const firstMarker = page
			.getByTestId("theme-section-key-points")
			.getByRole("button", {
				name: "Open source 1: Incremental Linker Benchmarks",
			})
			.first();
		await expect(firstMarker).toHaveText("[1]");
		await firstMarker.click();

		const overlay = page.getByTestId("reader-overlay");
		await expect(overlay).toBeVisible();
		await overlay.getByTestId("reader-overlay-close").click();
		await expect(overlay).toBeHidden();

		// Switch to version 1 and verify What changed & Open questions disappear
		await page
			.getByTestId("theme-section-versions")
			.getByRole("link", { name: /Version 1/ })
			.click();

		await expect(page.getByTestId("theme-meta")).toContainText(
			"Version 1 of 2 ·",
		);
		await expect(page.getByTestId("theme-older-notice")).toBeVisible();
		await expect(page.getByTestId("theme-section-what-changed")).toBeHidden();
		await expect(page.getByTestId("theme-section-open-questions")).toBeHidden();

		// Return to /knowledge and confirm " · new" is gone
		await page.getByTestId("theme-back-link").click();
		await expect.poll(() => new URL(page.url()).pathname).toBe("/knowledge");

		const updatedFirstRow = page.getByTestId("theme-row").nth(0);
		await expect(updatedFirstRow).toContainText("Compiler Toolchains");
		await expect(updatedFirstRow).not.toContainText("· new");
	});

	test("has no horizontal overflow at 360px and tap targets are at least 44px", async ({
		page,
	}) => {
		await page.setViewportSize({ width: 360, height: 800 });

		await gotoMobileRoute(page, "knowledge");
		await expect(page.getByTestId("theme-row")).toHaveCount(2);

		const listOverflow = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}));
		expect(listOverflow.scrollWidth).toBeLessThanOrEqual(
			listOverflow.clientWidth,
		);

		const rowLinkBox = await page
			.getByTestId("theme-row")
			.first()
			.getByRole("link")
			.boundingBox();
		expect(rowLinkBox).not.toBeNull();
		expect(rowLinkBox?.height).toBeGreaterThanOrEqual(44);

		const trailLinkBox = await page
			.getByTestId("knowledge-trail-link")
			.boundingBox();
		expect(trailLinkBox).not.toBeNull();
		expect(trailLinkBox?.height).toBeGreaterThanOrEqual(44);

		await gotoMobileRoute(page, `knowledge/themes/${THEME_UNOPENED_ID}`);
		await expect(page.getByTestId("theme-section-key-points")).toBeVisible();

		const detailOverflow = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}));
		expect(detailOverflow.scrollWidth).toBeLessThanOrEqual(
			detailOverflow.clientWidth,
		);

		const tapTargets = [
			page.getByTestId("theme-back-link"),
			page.getByTestId("theme-section-key-points").getByRole("button").first(),
			page.getByTestId("theme-source-row").first(),
			page.getByTestId("theme-section-versions").getByRole("link").first(),
		];

		for (const target of tapTargets) {
			const box = await target.boundingBox();
			expect(box).not.toBeNull();
			expect(box?.height).toBeGreaterThanOrEqual(44);
			expect(box?.width).toBeGreaterThanOrEqual(44);
		}
	});
});
