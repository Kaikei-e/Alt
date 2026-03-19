import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import StreamUpdateBar from "./StreamUpdateBar.svelte";

describe("StreamUpdateBar", () => {
	it("shows pending update count and apply button", async () => {
		render(StreamUpdateBar as never, {
			props: {
				pendingCount: 3,
				isConnected: true,
				isFallback: false,
				onApply: vi.fn(),
			},
		});

		await expect.element(page.getByText("3 items updated")).toBeInTheDocument();
	});

	it("shows singular for 1 item", async () => {
		render(StreamUpdateBar as never, {
			props: {
				pendingCount: 1,
				isConnected: true,
				isFallback: false,
				onApply: vi.fn(),
			},
		});

		await expect.element(page.getByText("1 item updated")).toBeInTheDocument();
	});

	it("shows disconnected status when not connected and no pending", async () => {
		render(StreamUpdateBar as never, {
			props: {
				pendingCount: 0,
				isConnected: false,
				isFallback: false,
				onApply: vi.fn(),
			},
		});

		await expect.element(page.getByText("Reconnecting...")).toBeInTheDocument();
	});

	it("shows fallback status", async () => {
		render(StreamUpdateBar as never, {
			props: {
				pendingCount: 0,
				isConnected: false,
				isFallback: true,
				onApply: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Live updates unavailable"))
			.toBeInTheDocument();
	});

	it("renders nothing when connected with no pending updates", async () => {
		const { container } = render(StreamUpdateBar as never, {
			props: {
				pendingCount: 0,
				isConnected: true,
				isFallback: false,
				onApply: vi.fn(),
			},
		});

		expect(container.textContent?.trim()).toBe("");
	});
});
