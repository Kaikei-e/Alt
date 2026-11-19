import fs from "node:fs";
import path from "node:path";
import type { Locator, Page, Route } from "@playwright/test";
import { expect } from "@playwright/test";

export const CAPTURE_NOW = "2026-11-20T09:30:00Z";
export const CAPTURE_OUTPUT_DIR = "test-results/readme-capture";

const CAPTURE_NOW_MS = Date.parse(CAPTURE_NOW);

export function minutesAgo(minutes: number): string {
	return new Date(CAPTURE_NOW_MS - minutes * 60_000).toISOString();
}

export function hoursAgo(hours: number): string {
	return new Date(CAPTURE_NOW_MS - hours * 3_600_000).toISOString();
}

export function daysAgo(days: number): string {
	return new Date(CAPTURE_NOW_MS - days * 86_400_000).toISOString();
}

export function hoursFromNow(hours: number): string {
	return new Date(CAPTURE_NOW_MS + hours * 3_600_000).toISOString();
}

export function captureDate(): string {
	return CAPTURE_NOW.slice(0, 10);
}

const SHARED_CONTEXT_OPTIONS = {
	colorScheme: "light" as const,
	reducedMotion: "reduce" as const,
	locale: "en-US",
	timezoneId: "UTC",
	storageState: "tests/e2e/.auth/storage.json",
};

export const DESKTOP_CONTEXT = {
	...SHARED_CONTEXT_OPTIONS,
	viewport: { width: 1440, height: 900 },
	deviceScaleFactor: 2,
};

export const PHONE_CONTEXT = {
	...SHARED_CONTEXT_OPTIONS,
	viewport: { width: 412, height: 732 },
	deviceScaleFactor: 2.625,
	isMobile: true,
	hasTouch: true,
	userAgent:
		"Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
};

const LOCAL_HOSTNAMES = new Set(["127.0.0.1", "localhost", "::1", "[::1]"]);
const FONT_HOSTNAMES = new Set(["fonts.googleapis.com", "fonts.gstatic.com"]);

export async function fulfillJson(
	route: Route,
	body: unknown,
	status = 200,
): Promise<void> {
	await route.fulfill({
		status,
		contentType: "application/json",
		body: JSON.stringify(body),
	});
}

export async function installCaptureNetworkGuard(page: Page): Promise<void> {
	await page.context().route(
		(url) => url.protocol === "http:" || url.protocol === "https:",
		async (route) => {
			const url = new URL(route.request().url());
			if (
				LOCAL_HOSTNAMES.has(url.hostname) ||
				FONT_HOSTNAMES.has(url.hostname)
			) {
				await route.continue();
				return;
			}
			await route.abort("blockedbyclient");
		},
	);
}

export async function prepareCapturePage(page: Page): Promise<void> {
	await installCaptureNetworkGuard(page);
	await page.clock.setFixedTime(new Date(CAPTURE_NOW));
}

const LOADING_INDICATORS_SELECTOR = [
	'[data-testid="system-loader"]:visible',
	'[data-testid="today-loading"]:visible',
	".skeleton-line:visible",
	".skel-card:visible",
	".skel-bar:visible",
	".loading-more:visible",
	".loading-pulse:visible",
	".loading-dot:visible",
	".thumbnail-shimmer:visible",
	".rail-skeleton:visible",
	".animate-spin:visible",
	".animate-pulse:visible",
	'[role="progressbar"]:visible',
].join(", ");

export async function captureSurface(
	page: Page,
	options: {
		filename: string;
		contentLocator: Locator;
		expectedTitles: string[];
		minVisibleTitles?: number;
	},
): Promise<void> {
	const minVisibleTitles = options.minVisibleTitles ?? 3;

	await page.locator("body.hydrated").waitFor({ state: "attached" });
	await page.evaluate(() => document.fonts.ready);
	await page.waitForFunction(() => {
		const splash = document.getElementById("splash");
		return !splash || getComputedStyle(splash).opacity === "0";
	});

	await options.contentLocator.waitFor({ state: "visible" });
	await expect(page.locator(LOADING_INDICATORS_SELECTOR)).toHaveCount(0);

	let visibleTitles = 0;
	for (const title of options.expectedTitles) {
		const matches = page.getByText(title);
		const count = await matches.count();
		for (let i = 0; i < count; i++) {
			if (await matches.nth(i).isVisible()) {
				visibleTitles++;
				break;
			}
		}
	}
	expect(visibleTitles).toBeGreaterThanOrEqual(minVisibleTitles);

	fs.mkdirSync(CAPTURE_OUTPUT_DIR, { recursive: true });
	await page.screenshot({
		path: path.join(CAPTURE_OUTPUT_DIR, options.filename),
		fullPage: false,
		animations: "disabled",
	});
}
