import { create } from "@bufbuild/protobuf";
import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import AdminMetricCard from "./AdminMetricCard.svelte";
import AdminTabNavigation from "./AdminTabNavigation.svelte";
import DiffSummaryPanel from "./DiffSummaryPanel.svelte";
import MetricRow from "./observability/MetricRow.svelte";
import ServiceHealthTable from "./observability/ServiceHealthTable.svelte";
import ReprojectActions from "./ReprojectActions.svelte";
import RetentionStatusPanel from "./RetentionStatusPanel.svelte";

const MOBILE_VIEWPORT = { width: 375, height: 667 };

describe("Knowledge Home Admin mobile layout (375px)", () => {
	it("AdminTabNavigation stays within 375px width and scrolls horizontally to reach Observability", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const onTabChange = vi.fn();
		const { container } = render(AdminTabNavigation, {
			props: { activeTab: "overview", onTabChange },
		});

		const nav = container.querySelector<HTMLElement>(
			'[data-role="admin-tab-nav"]',
		);
		expect(nav).not.toBeNull();
		if (!nav) throw new Error("expected admin-tab-nav");
		const navRect = nav.getBoundingClientRect();
		expect(navRect.right).toBeLessThanOrEqual(MOBILE_VIEWPORT.width + 1);
		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			MOBILE_VIEWPORT.width + 1,
		);

		const obsButton = page.getByRole("button", { name: "Observability" });
		await obsButton.click();
		expect(onTabChange).toHaveBeenCalledWith("observability");
	});

	it("AdminMetricCard wraps long table names and values inside narrow mobile columns", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const { container } = render(AdminMetricCard, {
			props: {
				label: "recall_candidate_view_partitioned_long_name",
				value: "knowledge_home v2",
				status: "ok",
			},
		});

		// Simulate a 2-column mobile grid cell (~165px wide)
		container.style.width = "165px";
		const card = container.querySelector<HTMLElement>(
			'[data-role="metric-card"]',
		);
		expect(card).not.toBeNull();
		if (!card) throw new Error("expected metric-card");
		expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth + 1);
	});

	it("ReprojectActions fits inside 375px and keeps mobile inputs >= 16px", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const { container } = render(ReprojectActions, {
			props: { onStart: vi.fn(), inFlight: false },
		});

		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			MOBILE_VIEWPORT.width + 1,
		);
		const inputs = container.querySelectorAll<HTMLInputElement>("input");
		expect(inputs.length).toBeGreaterThan(0);
		for (const input of inputs) {
			const rect = input.getBoundingClientRect();
			expect(rect.right).toBeLessThanOrEqual(MOBILE_VIEWPORT.width + 1);
			expect(
				Number.parseFloat(getComputedStyle(input).fontSize),
			).toBeGreaterThanOrEqual(16);
		}
	});

	it("RetentionStatusPanel wraps header actions and contains wide tables inside 375px", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const { container } = render(RetentionStatusPanel, {
			props: {
				eligiblePartitions: [
					{
						table: "knowledge_events",
						eligible: [
							{
								name: "knowledge_events_y2025m11",
								rangeStart: "2025-11-01T00:00:00Z",
								rangeEnd: "2025-12-01T00:00:00Z",
								rowCount: 42000,
								sizeBytes: 52428800,
							},
						],
					},
				],
				retentionLogs: [
					{
						logId: "log-1",
						runAt: "2026-11-27T10:00:00Z",
						action: "export",
						targetTable: "knowledge_events",
						targetPartition: "knowledge_events_y2025m11",
						rowsAffected: 42000,
						archivePath: "/tmp/archive.jsonl.gz",
						checksum: "sha256:abc",
						dryRun: false,
						status: "exported",
						errorMessage: "",
					},
				],
			},
		});

		// Wrap in a CSS grid like +page.svelte does; without min-width: 0 on .panel,
		// the grid expands to fit the wide table instead of scrolling .table-container.
		container.style.display = "grid";
		container.style.gridTemplateColumns = "1fr";
		container.style.width = `${MOBILE_VIEWPORT.width}px`;

		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			MOBILE_VIEWPORT.width + 1,
		);
		const header = container.querySelector<HTMLElement>(".panel-header");
		expect(header).not.toBeNull();
		if (!header) throw new Error("expected panel-header");
		expect(getComputedStyle(header).flexWrap).toBe("wrap");
	});

	it("DiffSummaryPanel stacks why-distribution columns cleanly on 375px", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const { container } = render(DiffSummaryPanel, {
			props: {
				diff: {
					fromItemCount: 14820,
					toItemCount: 15012,
					fromEmptyCount: 412,
					toEmptyCount: 12,
					fromAvgScore: 0.614,
					toAvgScore: 0.782,
					fromWhyDistribution:
						'{"new_unread_very_long_key_name":8200,"summary_completed":4200}',
					toWhyDistribution:
						'{"new_unread_very_long_key_name":8400,"summary_completed":4412}',
				},
			},
		});

		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			MOBILE_VIEWPORT.width + 1,
		);
		const dist = container.querySelector<HTMLElement>(".diff-dist");
		expect(dist).not.toBeNull();
		if (!dist) throw new Error("expected diff-dist");
		const cols = getComputedStyle(dist).gridTemplateColumns.split(" ");
		expect(cols).toHaveLength(1);
	});

	it("Observability ServiceHealthTable and MetricRow do not overflow 375px", async () => {
		await page.viewport(MOBILE_VIEWPORT.width, MOBILE_VIEWPORT.height);
		const availability = create(MetricResultSchema, {
			key: "availability_services",
			kind: SeriesKind.INSTANT,
			unit: "bool",
			series: [
				create(SeriesSchema, {
					labels: { job: "knowledge-sovereign" },
					points: [
						create(PointSchema, {
							time: "2026-11-27T12:34:56Z",
							value: 1,
						}),
					],
				}),
			],
		});
		const latency = create(MetricResultSchema, {
			key: "http_latency_p95",
			kind: SeriesKind.RANGE,
			unit: "seconds",
			grafanaUrl: "http://127.0.0.1:3000/d/test",
			series: [
				create(SeriesSchema, {
					labels: { job: "alt-butterfly-facade" },
					points: [
						create(PointSchema, { time: "2026-11-27T12:30:00Z", value: 0.15 }),
						create(PointSchema, { time: "2026-11-27T12:35:00Z", value: 0.35 }),
					],
				}),
				create(SeriesSchema, {
					labels: { job: "knowledge-sovereign" },
					points: [
						create(PointSchema, { time: "2026-11-27T12:30:00Z", value: 0.05 }),
						create(PointSchema, { time: "2026-11-27T12:35:00Z", value: 0.06 }),
					],
				}),
			],
		});

		render(ServiceHealthTable, { props: { availability } });
		render(MetricRow, {
			props: { metric: latency, label: "HTTP p95 latency" },
		});

		expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(
			MOBILE_VIEWPORT.width + 1,
		);
	});
});
