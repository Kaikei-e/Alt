import fs from "node:fs";
import path from "node:path";
import { expect, test } from "@playwright/test";
import {
	ACOLYTE_EXPECTED_TEXTS,
	ACOLYTE_REPORT_ID,
	mockAcolyteRoutes,
} from "./fixtures/acolyte";
import {
	AUGUR_CONVERSATION_ID,
	AUGUR_EXPECTED_TEXTS,
	mockAugurRoutes,
} from "./fixtures/augur";
import {
	KNOWLEDGE_HOME_TITLES,
	mockKnowledgeHomeRoutes,
} from "./fixtures/knowledgeHome";
import { mockTrailRoutes, TRAIL_TITLES } from "./fixtures/knowledgeTrail";
import {
	MORNING_LETTER_EXPECTED_TEXTS,
	mockMorningLetterRoutes,
} from "./fixtures/morningLetter";
import { mockSwipeRoutes, SWIPE_EXPECTED_TITLES } from "./fixtures/swipeDeck";
import { mockTodayRoutes, TODAY_QUEUE_TITLES } from "./fixtures/todayQueue";
import { browserFrame, frameDocument, phoneFrame } from "./frames";
import {
	CAPTURE_OUTPUT_DIR,
	CAPTURE_RAW_DIR,
	captureSurface,
	DESKTOP_CONTEXT,
	installCaptureNetworkGuard,
	PHONE_CONTEXT,
	prepareCapturePage,
} from "./helpers";

function readRawDataUri(filename: string): string {
	const filePath = path.join(CAPTURE_RAW_DIR, filename);
	return `data:image/png;base64,${fs.readFileSync(filePath).toString("base64")}`;
}

async function waitForStageAssets(
	page: import("@playwright/test").Page,
): Promise<void> {
	await page.evaluate(() => document.fonts.ready);
	await expect
		.poll(() =>
			page.evaluate(() => {
				const imgs = Array.from(document.querySelectorAll("img"));
				return (
					imgs.length > 0 &&
					imgs.every((img) => img.complete && img.naturalWidth > 0)
				);
			}),
		)
		.toBe(true);
}

const STAGE_CSS = `html, body {
	margin: 0;
	padding: 0;
	background: transparent;
}
.stage {
	display: inline-block;
	padding: 32px 32px 56px;
	background: transparent;
}
.browser-window, .phone-frame {
	box-shadow: 0 18px 36px -18px rgba(46, 40, 31, 0.35);
}`;

const HERO_CSS = `html, body {
	margin: 0;
	padding: 0;
	width: 1600px;
	height: 1000px;
	overflow: hidden;
}
body {
	position: relative;
	background: radial-gradient(120% 90% at 28% 12%, #f4f1ea 0%, #e8e3d9 55%, #ddd6ca 100%);
}
.hero-browser {
	position: absolute;
	left: 70px;
	top: 80px;
	box-shadow:
		0 34px 70px -24px rgba(46, 40, 31, 0.40),
		0 14px 28px -14px rgba(46, 40, 31, 0.28);
}
.hero-phone {
	position: absolute;
	right: 90px;
	bottom: 60px;
	box-shadow: 0 30px 60px -20px rgba(46, 40, 31, 0.55);
}`;

const DESKTOP_FRAMES = [
	{ filename: "trail-desktop.png", address: "localhost/knowledge/trail" },
	{ filename: "augur-desktop.png", address: "localhost/augur" },
	{
		filename: "morning-letter-desktop.png",
		address: "localhost/recap/morning-letter",
	},
	{ filename: "acolyte-desktop.png", address: "localhost/acolyte/reports" },
] as const;

const PHONE_FRAMES = [
	"today-phone.png",
	"swipe-phone.png",
	"trail-phone.png",
] as const;

test.describe("README Screenshot Captures", () => {
	test.describe("Knowledge Home (desktop)", () => {
		test.use(DESKTOP_CONTEXT);

		test("Knowledge Home on desktop", async ({ page }) => {
			await prepareCapturePage(page);
			const releaseStream = await mockKnowledgeHomeRoutes(page);

			try {
				await page.goto("/home");

				await expect(
					page.getByRole("heading", { level: 1, name: "Knowledge Home" }),
				).toBeVisible();
				await expect(page.getByText("Systems & Rust")).toBeVisible();
				await expect(page.getByText("AI Research")).toBeVisible();
				await expect(
					page.getByText(
						"Deterministic replay testing for Kubernetes controllers",
					),
				).toBeVisible();

				await captureSurface(page, {
					filename: "home-desktop.png",
					contentLocator: page.locator("article[data-item-key]").first(),
					expectedTitles: KNOWLEDGE_HOME_TITLES,
				});
			} finally {
				await releaseStream();
			}
		});
	});

	test.describe("Today (phone)", () => {
		test.use(PHONE_CONTEXT);

		test("Today on phone", async ({ page }) => {
			await prepareCapturePage(page);
			await mockTodayRoutes(page);

			await page.goto("/today");

			await expect(
				page.getByRole("heading", { level: 1, name: "Today" }),
			).toBeVisible();
			await expect(
				page.getByRole("navigation", { name: "Main navigation" }),
			).toBeVisible();
			await expect(page.getByTestId("today-queue-count")).toContainText(
				"5 / 7",
			);

			await captureSurface(page, {
				filename: "today-phone.png",
				contentLocator: page.getByTestId("today-queue-list"),
				expectedTitles: TODAY_QUEUE_TITLES,
			});
		});
	});

	test.describe("Knowledge Trail (desktop)", () => {
		test.use(DESKTOP_CONTEXT);

		test("Knowledge Trail on desktop", async ({ page }) => {
			await prepareCapturePage(page);
			await mockTrailRoutes(page);

			await page.goto("/knowledge/trail");

			await expect(
				page.getByRole("heading", { level: 1, name: "Your Trail" }),
			).toBeVisible();
			await expect(page.getByTestId("episode-branch")).toBeVisible();
			await expect(
				page.getByText("Tuning PostgreSQL vacuum workers for heavy writes"),
			).toBeVisible();

			await captureSurface(page, {
				filename: "trail-desktop.png",
				contentLocator: page.getByTestId("trail-episode").first(),
				expectedTitles: TRAIL_TITLES,
			});
		});
	});

	test.describe("Knowledge Trail (phone)", () => {
		test.use(PHONE_CONTEXT);

		test("Knowledge Trail on phone", async ({ page }) => {
			await prepareCapturePage(page);
			await mockTrailRoutes(page);

			await page.goto("/knowledge/trail");

			await expect(
				page.getByRole("heading", { level: 1, name: "Your Trail" }),
			).toBeVisible();
			await expect(page.getByTestId("episode-branch")).toBeVisible();

			await captureSurface(page, {
				filename: "trail-phone.png",
				contentLocator: page.getByTestId("trail-episode").first(),
				expectedTitles: TRAIL_TITLES,
			});
		});
	});

	test.describe("Ask Augur (desktop)", () => {
		test.use(DESKTOP_CONTEXT);

		test("Ask Augur on desktop", async ({ page }) => {
			await prepareCapturePage(page);
			await mockAugurRoutes(page);

			await page.goto(`/augur/${AUGUR_CONVERSATION_ID}`);

			await expect(page.locator(".citation-rail")).toBeVisible();
			await expect(page.locator(".augur-shell.revealed")).toBeVisible();

			await captureSurface(page, {
				filename: "augur-desktop.png",
				contentLocator: page.locator(".citation-rail"),
				expectedTitles: AUGUR_EXPECTED_TEXTS,
			});
		});
	});

	test.describe("Visual Preview Swipe (phone)", () => {
		test.use(PHONE_CONTEXT);

		test("Swipe visual preview on phone", async ({ page }) => {
			await prepareCapturePage(page);
			await mockSwipeRoutes(page);

			await page.goto("/today");
			await page
				.getByRole("navigation", { name: "Main navigation" })
				.getByRole("link", { name: "Swipe" })
				.click();
			await page.waitForURL(/\/feeds\/swipe\/visual-preview/);

			const card = page.getByTestId("visual-preview-card");
			await expect(card).toBeVisible();
			await page.getByTestId("swipe-pass-button").click();
			await expect(page.getByTestId("dispatch-counter")).toHaveText("№ 1");

			await expect(
				card.getByRole("heading", {
					name: "Designing editorial grids with variable display type",
				}),
			).toBeVisible();
			const coverImg = card.getByTestId("thumbnail-image");
			await expect(coverImg).toBeVisible();
			await expect
				.poll(() =>
					coverImg.evaluate(
						(el) =>
							el instanceof HTMLImageElement &&
							el.complete &&
							el.naturalWidth > 0,
					),
				)
				.toBe(true);
			await expect(page.getByTestId("swipe-pass-button")).toBeVisible();
			await expect(page.getByTestId("swipe-queue-button")).toBeVisible();

			await captureSurface(page, {
				filename: "swipe-phone.png",
				contentLocator: card,
				expectedTitles: SWIPE_EXPECTED_TITLES,
				minVisibleTitles: 1,
			});
		});
	});

	test.describe("Morning Letter (desktop)", () => {
		test.use(DESKTOP_CONTEXT);

		test("Morning Letter on desktop", async ({ page }) => {
			await prepareCapturePage(page);
			await mockMorningLetterRoutes(page);

			await page.goto("/recap/morning-letter");

			await expect(page.locator(".morning-letter-page.revealed")).toBeVisible();
			await expect(
				page.getByPlaceholder("Ask about this edition..."),
			).toBeVisible();

			await captureSurface(page, {
				filename: "morning-letter-desktop.png",
				contentLocator: page.locator(".letter-document"),
				expectedTitles: MORNING_LETTER_EXPECTED_TEXTS,
			});
		});
	});

	test.describe("Acolyte Report (desktop)", () => {
		test.use(DESKTOP_CONTEXT);

		test("Acolyte report detail on desktop", async ({ page }) => {
			await prepareCapturePage(page);
			await mockAcolyteRoutes(page);

			await page.goto(`/acolyte/reports/${ACOLYTE_REPORT_ID}`);

			await expect(page.locator(".aco-detail.revealed")).toBeVisible();
			await expect(page.getByRole("status")).toBeVisible();

			await captureSurface(page, {
				filename: "acolyte-desktop.png",
				contentLocator: page.locator(".section-article"),
				expectedTitles: ACOLYTE_EXPECTED_TEXTS,
			});
		});
	});

	test.describe("Frame Composition", () => {
		test.describe("Desktop frames", () => {
			test.use({
				viewport: { width: 1600, height: 1200 },
				deviceScaleFactor: 2,
				colorScheme: "light",
				contextOptions: { reducedMotion: "reduce" },
				locale: "en-US",
				timezoneId: "UTC",
			});

			for (const entry of DESKTOP_FRAMES) {
				test(`Compose browser frame for ${entry.filename}`, async ({
					page,
				}) => {
					await installCaptureNetworkGuard(page);

					const html = frameDocument({
						body: `<div class="stage">${browserFrame({
							src: readRawDataUri(entry.filename),
							address: entry.address,
							width: 1440,
						})}</div>`,
						extraCss: STAGE_CSS,
					});
					await page.setContent(html);
					await waitForStageAssets(page);

					fs.mkdirSync(CAPTURE_OUTPUT_DIR, { recursive: true });
					await page.locator(".stage").screenshot({
						path: path.join(CAPTURE_OUTPUT_DIR, entry.filename),
						omitBackground: true,
						animations: "disabled",
					});
				});
			}
		});

		test.describe("Phone frames", () => {
			test.use({
				viewport: { width: 600, height: 1100 },
				deviceScaleFactor: 3,
				colorScheme: "light",
				contextOptions: { reducedMotion: "reduce" },
				locale: "en-US",
				timezoneId: "UTC",
			});

			for (const filename of PHONE_FRAMES) {
				test(`Compose phone frame for ${filename}`, async ({ page }) => {
					await installCaptureNetworkGuard(page);

					const html = frameDocument({
						body: `<div class="stage">${phoneFrame({
							src: readRawDataUri(filename),
							screenWidth: 393,
						})}</div>`,
						extraCss: STAGE_CSS,
					});
					await page.setContent(html);
					await waitForStageAssets(page);

					fs.mkdirSync(CAPTURE_OUTPUT_DIR, { recursive: true });
					await page.locator(".stage").screenshot({
						path: path.join(CAPTURE_OUTPUT_DIR, filename),
						omitBackground: true,
						animations: "disabled",
					});
				});
			}
		});
	});

	test.describe("Hero Composition", () => {
		test.use({
			viewport: { width: 1600, height: 1000 },
			deviceScaleFactor: 2,
			colorScheme: "light",
			contextOptions: { reducedMotion: "reduce" },
			locale: "en-US",
			timezoneId: "UTC",
		});

		test("Compose hero image from desktop and phone captures", async ({
			page,
		}) => {
			await installCaptureNetworkGuard(page);

			const homeDataUri = readRawDataUri("home-desktop.png");
			const todayDataUri = readRawDataUri("today-phone.png");

			const html = frameDocument({
				body: `${browserFrame({
					src: homeDataUri,
					address: "localhost/home",
					width: 1250,
					className: "hero-browser",
				})}
${phoneFrame({
	src: todayDataUri,
	screenWidth: 270,
	className: "hero-phone",
})}`,
				extraCss: HERO_CSS,
			});
			await page.setContent(html);
			await waitForStageAssets(page);

			const phoneBox = await page.locator(".phone-frame").boundingBox();
			expect(phoneBox).not.toBeNull();
			if (phoneBox) {
				expect(phoneBox.x).toBeGreaterThanOrEqual(0);
				expect(phoneBox.y).toBeGreaterThanOrEqual(0);
				expect(phoneBox.x + phoneBox.width).toBeLessThanOrEqual(1600);
				expect(phoneBox.y + phoneBox.height).toBeLessThanOrEqual(1000);
			}

			fs.mkdirSync(CAPTURE_OUTPUT_DIR, { recursive: true });
			await page.screenshot({
				path: path.join(CAPTURE_OUTPUT_DIR, "hero.png"),
				fullPage: false,
				animations: "disabled",
			});
		});
	});
});
