import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";

import MetricSeriesDigest from "./MetricSeriesDigest.svelte";
import type { SimpleSeries } from "./util";

function series(label: string, values: number[]): SimpleSeries {
	return {
		labels: { job: label },
		points: values.map((v, i) => ({
			time: new Date(1_700_000_000_000 + i * 15_000).toISOString(),
			value: v,
		})),
	};
}

describe("MetricSeriesDigest", () => {
	it("renders top-N rows sorted by lead value", async () => {
		render(MetricSeriesDigest as never, {
			props: {
				series: [
					series("alpha", [1]),
					series("beta", [5]),
					series("gamma", [3]),
				],
				preferLabel: "job",
				unit: "req/s",
				limit: 3,
			},
		});
		await expect.element(page.getByText("beta")).toBeInTheDocument();
		await expect.element(page.getByText("alpha")).toBeInTheDocument();
		await expect.element(page.getByText("gamma")).toBeInTheDocument();
	});

	it("shows +N more when overflow exists", async () => {
		const s = Array.from({ length: 6 }, (_, i) => series(`svc-${i}`, [i]));
		render(MetricSeriesDigest as never, {
			props: { series: s, preferLabel: "job", limit: 3 },
		});
		await expect.element(page.getByText(/\+3 more/)).toBeInTheDocument();
	});

	it("renders em-dash when series is empty", async () => {
		render(MetricSeriesDigest as never, {
			props: { series: [], preferLabel: "job", limit: 3 },
		});
		await expect.element(page.getByLabelText(/No series/)).toBeInTheDocument();
	});
});
