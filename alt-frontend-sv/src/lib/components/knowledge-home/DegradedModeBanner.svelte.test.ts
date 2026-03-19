import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import DegradedModeBanner from "./DegradedModeBanner.svelte";

describe("DegradedModeBanner", () => {
	it("renders the degraded message when service quality is degraded", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "degraded",
			},
		});

		await expect
			.element(
				page.getByText(
					"Some data sources are temporarily unavailable. Showing partial results.",
				),
			)
			.toBeInTheDocument();
	});

	it("renders the fallback message when service quality is fallback", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "fallback",
			},
		});

		await expect
			.element(
				page.getByText(
					"Service is running in fallback mode. Some sections may be unavailable or stale.",
				),
			)
			.toBeInTheDocument();
	});

	it("renders nothing when service quality is full", async () => {
		const { container } = render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "full",
			},
		});

		expect(container.textContent?.trim()).toBe("");
	});

	it("shows dismiss button when onDismiss is provided", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "degraded",
				onDismiss: vi.fn(),
			},
		});

		await expect.element(page.getByTitle("Dismiss")).toBeInTheDocument();
	});

	it("does not show dismiss button when onDismiss is not provided", async () => {
		render(DegradedModeBanner as never, {
			props: {
				serviceQuality: "degraded",
			},
		});

		await expect.element(page.getByTitle("Dismiss")).not.toBeInTheDocument();
	});
});
