import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import AttentionSummary from "./AttentionSummary.svelte";

describe("AttentionSummary", () => {
	it("renders NO DATA target availability message when serviceRows is empty after snapshot", () => {
		const screen = render(AttentionSummary, {
			props: {
				items: [],
				serviceRows: [],
				alertRows: [],
				alertsAvailable: true,
			},
		});
		expect(screen.container.textContent).toContain(
			"NO DATA — target availability unavailable",
		);
		expect(screen.container.textContent).not.toContain(
			"No snapshot received yet",
		);
	});
});
