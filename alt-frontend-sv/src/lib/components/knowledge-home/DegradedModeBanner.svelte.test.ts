import { render } from "vitest-browser-svelte";
import { page } from "vitest/browser";
import { describe, expect, it } from "vitest";
import DegradedModeBanner from "./DegradedModeBanner.svelte";

describe("DegradedModeBanner", () => {
	it("renders the degraded message when service quality is degraded", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "degraded",
			},
		});

		await expect
			.element(page.getByText("Some data sources are temporarily unavailable. Showing partial results."))
			.toBeInTheDocument();
	});

	it("renders the fallback message when service quality is fallback", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "fallback",
			},
		});

		await expect
			.element(page.getByText("Service is running in fallback mode. Showing cached snapshot. Some features may be unavailable."))
			.toBeInTheDocument();
	});
});
