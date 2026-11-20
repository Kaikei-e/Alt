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
import {
	CAPTURE_OUTPUT_DIR,
	captureSurface,
	DESKTOP_CONTEXT,
	installCaptureNetworkGuard,
	PHONE_CONTEXT,
	prepareCapturePage,
} from "./helpers";

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

			const homePath = path.join(CAPTURE_OUTPUT_DIR, "home-desktop.png");
			const todayPath = path.join(CAPTURE_OUTPUT_DIR, "today-phone.png");
			const homeDataUri = `data:image/png;base64,${fs.readFileSync(homePath).toString("base64")}`;
			const todayDataUri = `data:image/png;base64,${fs.readFileSync(todayPath).toString("base64")}`;

			await page.setContent(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Playfair+Display:ital,wght@0,400;0,600;0,700;0,800;1,400&family=Source+Sans+3:wght@300;400;500;600;700&display=swap" rel="stylesheet" />
    <style>
      * { box-sizing: border-box; }
      html, body {
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
      .browser-window {
        position: absolute;
        left: 70px;
        top: 80px;
        width: 1250px;
        border-radius: 12px;
        border: 1px solid rgba(40, 35, 28, 0.16);
        box-shadow:
          0 34px 70px -24px rgba(46, 40, 31, 0.40),
          0 14px 28px -14px rgba(46, 40, 31, 0.28);
        overflow: hidden;
        background: #faf9f7;
      }
      .title-bar {
        position: relative;
        height: 40px;
        background: linear-gradient(180deg, #efedea 0%, #e7e4de 100%);
        border-bottom: 1px solid #d6d2cb;
        display: flex;
        align-items: center;
        padding: 0 16px;
      }
      .dots {
        display: flex;
        align-items: center;
        gap: 8px;
      }
      .dot {
        width: 11px;
        height: 11px;
        border-radius: 50%;
        background: #cfcac1;
      }
      .address-pill {
        position: absolute;
        left: 50%;
        transform: translateX(-50%);
        background: #faf9f7;
        border: 1px solid #d6d2cb;
        border-radius: 7px;
        font-family: "IBM Plex Mono", monospace;
        font-size: 12px;
        line-height: 1.4;
        color: #5a564f;
        padding: 3px 28px;
        letter-spacing: 0.01em;
      }
      .browser-body img {
        display: block;
        width: 100%;
        height: auto;
      }
      .phone-frame {
        position: absolute;
        right: 90px;
        bottom: 60px;
        padding: 9px;
        background: #26231f;
        border-radius: 40px;
        box-shadow: 0 30px 60px -20px rgba(46, 40, 31, 0.55);
      }
      .phone-frame img {
        display: block;
        width: 280px;
        height: auto;
        border-radius: 31px;
      }
    </style>
  </head>
  <body>
    <div class="browser-window">
      <div class="title-bar">
        <div class="dots">
          <span class="dot"></span>
          <span class="dot"></span>
          <span class="dot"></span>
        </div>
        <div class="address-pill">localhost/home</div>
      </div>
      <div class="browser-body">
        <img id="hero-desktop" src="${homeDataUri}" alt="" />
      </div>
    </div>
    <div class="phone-frame">
      <img id="hero-phone" src="${todayDataUri}" alt="" />
    </div>
  </body>
</html>`);

			await page.evaluate(() => document.fonts.ready);
			await expect
				.poll(() =>
					page.evaluate(() => {
						const imgs = Array.from(document.querySelectorAll("img"));
						return imgs.every((img) => img.complete && img.naturalWidth > 0);
					}),
				)
				.toBe(true);

			await page.screenshot({
				path: path.join(CAPTURE_OUTPUT_DIR, "hero.png"),
				fullPage: false,
				animations: "disabled",
			});
		});
	});
});
