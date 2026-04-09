import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteHistorySheet from "./MobileAcolyteHistorySheet.svelte";
import { MOCK_VERSIONS } from "./acolyte-fixtures";

describe("MobileAcolyteHistorySheet", () => {
	it("renders Editions heading when open", async () => {
		render(MobileAcolyteHistorySheet as never, {
			props: {
				open: true,
				versions: MOCK_VERSIONS,
				onClose: vi.fn(),
			},
		});

		await expect.element(page.getByText("Editions")).toBeInTheDocument();
	});

	it("renders version items with edition number", async () => {
		render(MobileAcolyteHistorySheet as never, {
			props: {
				open: true,
				versions: MOCK_VERSIONS,
				onClose: vi.fn(),
			},
		});

		await expect.element(page.getByText("Ed. 2")).toBeInTheDocument();
		await expect.element(page.getByText("Ed. 1")).toBeInTheDocument();
	});

	it("renders change reason", async () => {
		render(MobileAcolyteHistorySheet as never, {
			props: {
				open: true,
				versions: MOCK_VERSIONS,
				onClose: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Full pipeline run"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Initial generation"))
			.toBeInTheDocument();
	});

	it("renders change tags with field names", async () => {
		render(MobileAcolyteHistorySheet as never, {
			props: {
				open: true,
				versions: MOCK_VERSIONS,
				onClose: vi.fn(),
			},
		});

		await expect.element(page.getByText("overview")).toBeInTheDocument();
		await expect.element(page.getByText("market_trends")).toBeInTheDocument();
	});

	it("shows empty state when no versions", async () => {
		render(MobileAcolyteHistorySheet as never, {
			props: {
				open: true,
				versions: [],
				onClose: vi.fn(),
			},
		});

		await expect
			.element(page.getByText(/no versions recorded/i))
			.toBeInTheDocument();
	});
});
