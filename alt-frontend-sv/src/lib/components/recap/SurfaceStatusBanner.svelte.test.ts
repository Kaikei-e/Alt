import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import SurfaceStatusBanner from "./SurfaceStatusBanner.svelte";

describe("SurfaceStatusBanner", () => {
	it("renders nothing when surfaceStatus is null or fresh without degradation", () => {
		const empty = render(SurfaceStatusBanner, {
			props: { surfaceStatus: null },
		});
		expect(empty.container.textContent?.trim()).toBe("");
		empty.unmount();

		const fresh = render(SurfaceStatusBanner, {
			props: {
				surfaceStatus: {
					generatedAt: "2026-09-22T17:00:00Z",
					windowStart: "2026-09-19T17:00:00Z",
					windowEnd: "2026-09-22T17:00:00Z",
					isStale: false,
					staleAfter: "2026-09-23T23:00:00Z",
					latestRun: {
						jobId: "run-1",
						status: "completed",
						kickedAt: "2026-09-22T17:00:00Z",
						updatedAt: "2026-09-22T17:05:00Z",
					},
					degradationReasons: [],
				},
			},
		});
		expect(fresh.container.textContent?.trim()).toBe("");
	});

	it("renders role=status info banner when latestRun is running", async () => {
		render(SurfaceStatusBanner, {
			props: {
				surfaceStatus: {
					generatedAt: "2026-09-22T17:00:00Z",
					windowStart: "2026-09-19T17:00:00Z",
					windowEnd: "2026-09-22T17:00:00Z",
					isStale: false,
					staleAfter: "2026-09-23T23:00:00Z",
					latestRun: {
						jobId: "run-running",
						status: "running",
						kickedAt: "2026-09-22T19:00:00Z",
						updatedAt: "2026-09-22T19:01:00Z",
					},
					degradationReasons: [],
				},
			},
		});

		await expect
			.element(page.getByRole("status"))
			.toHaveTextContent("Update in progress…");
	});

	it("renders role=alert error banner when latestRun is failed", async () => {
		render(SurfaceStatusBanner, {
			props: {
				surfaceStatus: {
					generatedAt: "2026-09-22T12:00:00Z",
					windowStart: "2026-09-19T12:00:00Z",
					windowEnd: "2026-09-22T12:00:00Z",
					isStale: false,
					staleAfter: "2026-09-23T18:00:00Z",
					latestRun: {
						jobId: "run-failed",
						status: "failed",
						kickedAt: "2026-09-22T19:00:00Z",
						updatedAt: "2026-09-22T19:02:00Z",
					},
					degradationReasons: [],
				},
			},
		});

		const alertEl = page.getByRole("alert");
		await expect
			.element(alertEl)
			.toHaveTextContent("The latest update failed at");
		await expect.element(alertEl).toHaveTextContent("Showing results from");
	});

	it("renders role=alert stale banner when isStale is true and not failed", async () => {
		render(SurfaceStatusBanner, {
			props: {
				surfaceStatus: {
					generatedAt: "2026-09-20T12:00:00Z",
					windowStart: "2026-09-17T12:00:00Z",
					windowEnd: "2026-09-20T12:00:00Z",
					isStale: true,
					staleAfter: "2026-09-21T18:00:00Z",
					latestRun: {
						jobId: "run-old",
						status: "completed",
						kickedAt: "2026-09-20T11:55:00Z",
						updatedAt: "2026-09-20T12:00:00Z",
					},
					degradationReasons: [],
				},
			},
		});

		const alertEl = page.getByRole("alert");
		await expect.element(alertEl).toHaveTextContent("This was last updated");
		await expect
			.element(alertEl)
			.toHaveTextContent("a newer update has not completed.");
	});

	it("renders degradation notices with role=alert", async () => {
		render(SurfaceStatusBanner, {
			props: {
				surfaceStatus: {
					generatedAt: "2026-09-22T17:00:00Z",
					windowStart: "2026-09-19T17:00:00Z",
					windowEnd: "2026-09-22T17:00:00Z",
					isStale: false,
					staleAfter: "2026-09-23T23:00:00Z",
					degradationReasons: [
						{
							code: "extractive_genres",
							detail:
								"9 of 19 genres are extractive excerpts, not AI summaries",
						},
					],
				},
			},
		});

		const alertEl = page.getByRole("alert");
		await expect
			.element(alertEl)
			.toHaveTextContent(
				"9 of 19 genres are extractive excerpts, not AI summaries",
			);
	});
});
