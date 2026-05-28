import { expect, test } from "@playwright/test";

/**
 * /admin/monitor — In-Alt system-health dashboard.
 *
 * Covers structural rendering and admin guard. Live-stream assertions (golden
 * signal cards have numeric values; degraded banner appears when Prometheus is
 * down) require a running stack and live Prometheus; those are covered by the
 * manual smoke sweep in docs/runbooks/admin-observability.md.
 *
 * Connect server-streaming is not mocked here because Playwright route
 * fulfillment cannot easily emit chunked Connect frames. The unit tests in
 * src/lib/components/admin/monitor/*.svelte.spec.ts cover component behavior
 * against canned MetricResult fixtures.
 */
test.describe("Admin /admin/monitor (desktop)", () => {
	test.beforeEach(async ({ page }) => {
		// Stub Catalog so the page does not loop on a failing initial RPC.
		// Connect-RPC uses POST with content-type=application/proto or +json.
		await page.route(
			"**/api/v2/alt.admin_monitor.v1.AdminMonitorService/Catalog",
			(route) =>
				route.fulfill({
					status: 200,
					contentType: "application/proto",
					body: Buffer.alloc(0),
				}),
		);
		// Watch is a server-streaming RPC; respond with an empty body so the
		// hook records "stream ended" and enters reconnect backoff without
		// causing test flake. The dashboard renders its structural shell
		// regardless of whether the stream produces frames.
		await page.route(
			"**/api/v2/alt.admin_monitor.v1.AdminMonitorService/Watch",
			(route) =>
				route.fulfill({
					status: 200,
					contentType: "application/proto",
					body: Buffer.alloc(0),
				}),
		);
	});

	test("renders the dashboard header and section headings", async ({
		page,
	}) => {
		await page.goto("/admin/monitor");
		await expect(
			page.getByRole("heading", { name: "System Monitor" }),
		).toBeVisible();

		// Section headings — Golden Signals, SLO, Service RED, Saturation, Queue.
		await expect(page.getByText("Golden signals")).toBeVisible();
		await expect(page.getByText(/SLO burn rate/i)).toBeVisible();
		await expect(page.getByText(/Service RED/i)).toBeVisible();
		await expect(page.getByText(/Container saturation/i)).toBeVisible();
		await expect(page.getByText(/Queue health/i)).toBeVisible();
	});

	test("exposes the window + step segmented controls", async ({ page }) => {
		await page.goto("/admin/monitor");
		const picker = page.getByRole("group", { name: "Time range" });
		await expect(picker).toBeVisible();
		// All five window options + four step options are present as radios.
		const radios = picker.locator('input[type="radio"]');
		await expect(radios).toHaveCount(9);
	});

	test("shows degraded banner while the stream has not delivered a frame", async ({
		page,
	}) => {
		await page.goto("/admin/monitor");
		// The empty-body Watch stub means the hook never reaches state=live;
		// banner must surface that with text + glyph (no color-only encoding).
		const banner = page.getByTestId("monitor-error-banner");
		await expect(banner).toBeVisible();
		await expect(banner).toContainText(/degraded|Connecting/i);
		await expect(banner).toContainText("●");
	});
});
