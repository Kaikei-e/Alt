import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	buildAuditWatchResponse,
	buildNoRulesWatchResponse,
	buildStaleWatchResponse,
	fulfillWatchStream,
} from "./admin-monitor.fixture";

const CATALOG_URL =
	"**/api/v2/alt.admin_monitor.v1.AdminMonitorService/Catalog";
const WATCH_URL = "**/api/v2/alt.admin_monitor.v1.AdminMonitorService/Watch";

test.describe("Admin /admin/monitor operator audit page", () => {
	test.skip(
		({ browserName }) => browserName !== "chromium",
		"Only Chromium browser binaries are installed on this host",
	);

	test.beforeEach(async ({ page }) => {
		await page.route(CATALOG_URL, (route) =>
			fulfillJson(route, { entries: [] }),
		);
	});

	test("renders audit sections top-to-bottom from Watch stream and clamps step on 24h", async ({
		page,
	}) => {
		await page.route(WATCH_URL, (route) =>
			fulfillWatchStream(route, [buildAuditWatchResponse()]),
		);

		await page.goto("/admin/monitor");

		await expect(
			page.getByRole("heading", { name: "System Monitor" }),
		).toBeVisible();
		await expect(page.getByTestId("header-timezone")).toContainText(
			/Times in \S+/,
		);

		const attention = page.locator("#attention");
		await expect(attention).toBeVisible();
		await expect(attention).toContainText("CRIT");
		await expect(attention).toContainText(/Edge SLO page burn active/i);
		await expect(attention).toContainText(
			"KnowledgeLoopRecapTopicSnapshotProducerDead",
		);
		await expect(attention).not.toContainText("Watchdog");

		const alerts = page.locator("#alerts");
		await expect(alerts).toBeVisible();
		const alertsTable = alerts.locator("table");
		await expect(alertsTable).toContainText(
			"KnowledgeLoopRecapTopicSnapshotProducerDead",
		);
		await expect(alertsTable).toContainText("knowledge-sovereign");
		await expect(alertsTable).not.toContainText("Watchdog");
		const heartbeat = alerts.getByTestId("alerts-heartbeat");
		await expect(heartbeat).toContainText("Watchdog");
		await expect(heartbeat).toContainText(/heartbeat/i);

		const edge = page.locator("#edge");
		await expect(edge).toBeVisible();
		await expect(edge).toContainText("SLO 99.9% availability at the edge");
		await expect(edge).toContainText("warn ≥ 0.10% · crit ≥ 1.44%");

		const services = page.locator("#services");
		await expect(services).toBeVisible();
		const newsCreatorRow = services.locator('tr[data-service="news-creator"]');
		await expect(newsCreatorRow).toBeVisible();
		await expect(newsCreatorRow).toContainText("1/2");
		await expect(newsCreatorRow).toContainText("CRIT");

		const recapEvalRow = services.locator('tr[data-service="recap-evaluator"]');
		await expect(recapEvalRow).toBeVisible();
		await expect(recapEvalRow).toContainText("no request metrics");

		const newsToggleBtn = newsCreatorRow.locator("button[aria-expanded]");
		await expect(newsToggleBtn).toHaveAttribute("aria-expanded", "false");
		await newsToggleBtn.focus();
		await page.keyboard.press("Enter");
		await expect(newsToggleBtn).toHaveAttribute("aria-expanded", "true");
		const drilldown = services.locator("#drill-news-creator");
		await expect(drilldown).toBeVisible();
		await expect(drilldown).toContainText("1/2 targets down");
		await expect(drilldown).toContainText("18 ms");
		await expect(drilldown).toContainText(/min .* max/i);

		const resources = page.locator("#resources");
		await expect(resources).toBeVisible();
		await expect(resources).toContainText("/dev/nvme0n1p2");
		await expect(resources.getByTestId("per-container-note")).toContainText(
			/Per-container breakdown is unavailable under the hardened cAdvisor configuration/i,
		);

		const certificates = page.locator("#certificates");
		await expect(certificates).toBeVisible();
		await expect(certificates).toContainText("auth-hub");

		const pipelines = page.locator("#pipelines");
		await expect(pipelines).toBeVisible();
		await expect(pipelines).toContainText("notification_outbox");

		const picker = page.getByRole("group", { name: "Time range" });
		await picker.getByText("24h", { exact: true }).click();

		const step15s = picker.locator('input[name="monitor-step"][value="1"]');
		const step30s = picker.locator('input[name="monitor-step"][value="2"]');
		const step1m = picker.locator('input[name="monitor-step"][value="3"]');
		const step5m = picker.locator('input[name="monitor-step"][value="4"]');

		await expect(step15s).toBeDisabled();
		await expect(step30s).toBeDisabled();
		await expect(step1m).toBeDisabled();
		await expect(step5m).toBeEnabled();
		await expect(step5m).toBeChecked();

		await page.setViewportSize({ width: 390, height: 844 });
		const overflowingChildren = await page.evaluate(() => {
			const root = document.querySelector("main.monitor");
			if (!root) return ["missing main.monitor"];
			const vw = window.innerWidth;
			const bad: string[] = [];
			for (const child of Array.from(root.children)) {
				const rect = child.getBoundingClientRect();
				if (rect.right > vw + 0.5 || rect.left < -0.5) {
					bad.push(
						`${child.tagName.toLowerCase()}#${child.id || child.className}: right=${rect.right} vw=${vw}`,
					);
				}
			}
			if (document.documentElement.scrollWidth > vw) {
				bad.push(
					`documentElement scrollWidth=${document.documentElement.scrollWidth} > vw=${vw}`,
				);
			}
			return bad;
		});
		expect(overflowingChildren).toEqual([]);
	});

	test("shows Waiting for first snapshot and non-OK verdict before frames arrive, preserves snapshot on Pause, and marks STALE after 15 s", async ({
		page,
	}) => {
		const baseIso = "2026-11-06T12:00:00.000Z";
		await page.clock.install({ time: new Date(baseIso) });

		let mode: "empty" | "single" = "empty";
		let deliveredSingle = false;

		await page.route(WATCH_URL, (route) => {
			if (mode === "single" && !deliveredSingle) {
				deliveredSingle = true;
				return fulfillWatchStream(route, [buildAuditWatchResponse(baseIso)]);
			}
			return fulfillWatchStream(route, []);
		});

		await page.goto("/admin/monitor");

		await expect(page.getByText("Waiting for first snapshot")).toBeVisible();
		const attention = page.locator("#attention");
		await expect(attention).toContainText("No snapshot received yet");
		await expect(attention).not.toContainText("All checks passing");
		await expect(attention.locator(".verdict-badge")).not.toContainText("OK");
		await expect(attention).toContainText("alerts unknown");

		const alerts = page.locator("#alerts");
		await expect(alerts).toContainText("NO DATA");
		await expect(alerts).not.toContainText("No firing or pending alerts.");

		mode = "single";
		await page.clock.runFor(1_200);

		await expect(page.getByText(/Updated .* · \d+s ago/)).toBeVisible();
		await expect(page.getByTestId("stale-badge")).toHaveCount(0);

		await page.getByRole("button", { name: "Pause" }).click();
		await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();
		await expect(page.getByText(/Updated .* · \d+s ago/)).toBeVisible();
		await expect(attention).toContainText("Edge SLO page burn active");

		await page.clock.runFor(16_000);

		await expect(page.getByTestId("stale-badge")).toBeVisible();
		await expect(page.getByTestId("stale-badge")).toContainText("STALE");
	});

	test("never renders an OK verdict when record-backed keys have zero series or stale points", async ({
		page,
	}) => {
		let mode: "norules" | "stale" = "norules";
		await page.route(WATCH_URL, (route) =>
			fulfillWatchStream(route, [
				mode === "norules"
					? buildNoRulesWatchResponse()
					: buildStaleWatchResponse(),
			]),
		);

		await page.goto("/admin/monitor");
		const attention = page.locator("#attention");
		await expect(attention).toBeVisible();
		await expect(attention).toContainText(
			"No current samples for edge_rps, host_cpu_utilization, host_memory_utilization, http_rps",
		);
		await expect(attention.locator(".verdict-badge")).toContainText("WARN");
		await expect(attention.locator(".verdict-badge")).not.toContainText("OK");
		await expect(attention).not.toContainText("All checks passing");

		mode = "stale";
		await page.reload();
		await expect(attention).toContainText(
			"No current samples for edge_rps, host_cpu_utilization, host_memory_utilization, http_rps",
		);
		await expect(attention.locator(".verdict-badge")).toContainText("WARN");
		await expect(attention.locator(".verdict-badge")).not.toContainText("OK");
		await expect(attention).not.toContainText("All checks passing");
	});
});
