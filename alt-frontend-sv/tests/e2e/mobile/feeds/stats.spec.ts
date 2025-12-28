import { test, expect, type Route } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

const STATS_RESPONSE = {
	feed_amount: { amount: 12 },
	total_articles: { amount: 345 },
	unsummarized_articles: { amount: 7 },
};

const UNREAD_RESPONSE = {
	count: 42,
};

const fulfillJson = async (
	route: Route,
	body: unknown,
	status: number = 200,
) => {
	await route.fulfill({
		status,
		contentType: "application/json",
		body: JSON.stringify(body),
	});
};

test.describe("mobile feeds routes - stats", () => {
	test("stats page renders mocked counters", async ({ page }) => {
		await page.addInitScript(() => {
			class MockEventSource {
				url: string;
				withCredentials = false;
				readyState = 1;

				static CONNECTING = 0;
				static OPEN = 1;
				static CLOSED = 2;

				readonly CONNECTING = 0;
				readonly OPEN = 1;
				readonly CLOSED = 2;

				onopen: ((this: EventSource, ev: Event) => void) | null = null;
				onmessage: ((this: EventSource, ev: MessageEvent) => void) | null =
					null;
				onerror: ((this: EventSource, ev: Event) => void) | null = null;

				constructor(url: string) {
					this.url = url;
					setTimeout(() => {
						this.onopen?.(new Event("open"));
					}, 0);
				}

				close() {
					this.readyState = 2;
				}

				addEventListener() {}
				removeEventListener() {}
				dispatchEvent() {
					return false;
				}
			}

			// @ts-expect-error - override EventSource for E2E stability.
			window.EventSource = MockEventSource;
		});

		await page.route("**/api/v1/feeds/stats/detailed", (route) =>
			fulfillJson(route, STATS_RESPONSE),
		);
		await page.route("**/api/v1/feeds/count/unreads", (route) =>
			fulfillJson(route, UNREAD_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/stats");

		await expect(page.getByText("Total Feeds")).toBeVisible();
		await expect(page.getByText("12")).toBeVisible();
		await expect(page.getByText("345")).toBeVisible();
		await expect(page.getByText("7")).toBeVisible();
		await expect(page.getByText("Today's Unread")).toBeVisible();
		await expect(page.getByText("42")).toBeVisible();
	});
});
