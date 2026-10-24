import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { LensData } from "#lib/connect/knowledge_home.js";
import LensSelector from "./LensSelector.svelte";

function makeLens(overrides: Partial<LensData> = {}): LensData {
	return {
		lensId: "lens-1",
		name: "AI News",
		description: "AI articles only",
		createdAt: "2026-03-17T10:00:00Z",
		updatedAt: "2026-03-17T10:00:00Z",
		currentVersion: {
			versionId: "v1",
			queryText: "agents",
			tagIds: ["AI"],
			sourceIds: [],
			timeWindow: "",
			includeRecap: false,
			includePulse: false,
			sortMode: "score",
		},
		...overrides,
	};
}

describe("LensSelector", () => {
	it("renders All button and lens buttons", async () => {
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: null,
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("All")).toBeInTheDocument();
		await expect.element(page.getByText("AI News")).toBeInTheDocument();
	});

	it("highlights active lens", async () => {
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: "lens-1",
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Active lens: AI News"))
			.toBeInTheDocument();
	});

	it("shows match count when provided for active lens", async () => {
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: "lens-1",
				matchCount: 15,
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("15 matches")).toBeInTheDocument();
	});

	it("does not show match count when no lens is active", async () => {
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: null,
				matchCount: 15,
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("15 matches")).not.toBeInTheDocument();
	});

	it("shows Save view button", async () => {
		render(LensSelector, {
			props: {
				lenses: [],
				activeLensId: null,
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Save current view"))
			.toBeInTheDocument();
	});

	it("shows active lens search summary when query is present", async () => {
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: "lens-1",
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
			},
		});

		await expect
			.element(page.getByText('Search: "agents"'))
			.toBeInTheDocument();
	});

	it("requires two-step inline confirm on per-pill delete and cancels cleanly", async () => {
		const onDelete = vi.fn();
		const confirmSpy = vi.spyOn(window, "confirm");
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: null,
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
				onDelete,
			},
		});

		const deletePillBtn = page.getByRole("button", {
			name: "Delete lens AI News",
		});
		await deletePillBtn.click();

		expect(onDelete).not.toHaveBeenCalled();
		expect(confirmSpy).not.toHaveBeenCalled();
		await expect.element(page.getByText("Delete?")).toBeInTheDocument();
		const confirmBtn = page.getByRole("button", {
			name: "Confirm delete lens AI News",
			exact: true,
		});
		const cancelBtn = page.getByRole("button", {
			name: "Cancel delete lens AI News",
			exact: true,
		});
		await expect.element(confirmBtn).toBeInTheDocument();
		await expect.element(cancelBtn).toBeInTheDocument();
		await expect.element(confirmBtn).toHaveFocus();

		await cancelBtn.click();
		expect(onDelete).not.toHaveBeenCalled();
		await expect.element(page.getByText("Delete?")).not.toBeInTheDocument();

		await deletePillBtn.click();
		await page
			.getByRole("button", {
				name: "Confirm delete lens AI News",
				exact: true,
			})
			.click();
		expect(onDelete).toHaveBeenCalledTimes(1);
		expect(onDelete).toHaveBeenCalledWith("lens-1");
		confirmSpy.mockRestore();
	});

	it("requires two-step inline confirm on active-lens summary Delete and cancels cleanly", async () => {
		const onDelete = vi.fn();
		const confirmSpy = vi.spyOn(window, "confirm");
		render(LensSelector, {
			props: {
				lenses: [makeLens()],
				activeLensId: "lens-1",
				onSelect: vi.fn(),
				onCreateClick: vi.fn(),
				onDelete,
			},
		});

		const summaryDeleteBtn = page.getByRole("button", {
			name: "Delete",
			exact: true,
		});
		await summaryDeleteBtn.click();

		expect(onDelete).not.toHaveBeenCalled();
		expect(confirmSpy).not.toHaveBeenCalled();
		await expect.element(page.getByText("Delete?")).toBeInTheDocument();
		const confirmBtn = page.getByRole("button", {
			name: "Confirm delete lens AI News",
			exact: true,
		});
		const cancelBtn = page.getByRole("button", {
			name: "Cancel delete lens AI News",
			exact: true,
		});
		await expect.element(confirmBtn).toBeInTheDocument();
		await expect.element(cancelBtn).toBeInTheDocument();
		await expect.element(confirmBtn).toHaveFocus();

		await cancelBtn.click();
		expect(onDelete).not.toHaveBeenCalled();
		await expect.element(page.getByText("Delete?")).not.toBeInTheDocument();

		await summaryDeleteBtn.click();
		await page
			.getByRole("button", {
				name: "Confirm delete lens AI News",
				exact: true,
			})
			.click();
		expect(onDelete).toHaveBeenCalledTimes(1);
		expect(onDelete).toHaveBeenCalledWith("lens-1");
		confirmSpy.mockRestore();
	});

	it("disarms summary delete confirm when switching to another lens", async () => {
		const lenses = [
			makeLens({ lensId: "lens-rust", name: "Rust" }),
			makeLens({ lensId: "lens-ai", name: "AI" }),
		];
		const onSelect = vi.fn();
		const onDelete = vi.fn();

		const { rerender } = render(LensSelector, {
			props: {
				lenses,
				activeLensId: "lens-rust",
				onSelect,
				onCreateClick: vi.fn(),
				onDelete,
			},
		});

		await page.getByRole("button", { name: "Delete", exact: true }).click();
		await expect
			.element(
				page.getByRole("button", {
					name: "Confirm delete lens Rust",
					exact: true,
				}),
			)
			.toBeInTheDocument();

		await page.getByRole("button", { name: "AI", exact: true }).click();
		expect(onSelect).toHaveBeenCalledWith("lens-ai");

		await rerender({
			lenses,
			activeLensId: "lens-ai",
			onSelect,
			onCreateClick: vi.fn(),
			onDelete,
		});

		await expect.element(page.getByText("Active lens: AI")).toBeInTheDocument();
		await expect.element(page.getByText("Delete?")).not.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /Confirm/ }))
			.not.toBeInTheDocument();
		expect(onDelete).not.toHaveBeenCalled();
	});

	it("disarms summary delete confirm when clicking All and then picking a lens", async () => {
		const lenses = [
			makeLens({ lensId: "lens-rust", name: "Rust" }),
			makeLens({ lensId: "lens-ai", name: "AI" }),
		];
		const onSelect = vi.fn();
		const onDelete = vi.fn();

		const { rerender } = render(LensSelector, {
			props: {
				lenses,
				activeLensId: "lens-rust",
				onSelect,
				onCreateClick: vi.fn(),
				onDelete,
			},
		});

		await page.getByRole("button", { name: "Delete", exact: true }).click();
		await expect.element(page.getByText("Delete?")).toBeInTheDocument();

		await page.getByRole("button", { name: "All" }).click();
		expect(onSelect).toHaveBeenCalledWith(null);
		await rerender({
			lenses,
			activeLensId: null,
			onSelect,
			onCreateClick: vi.fn(),
			onDelete,
		});

		await page.getByRole("button", { name: "Rust", exact: true }).click();
		expect(onSelect).toHaveBeenCalledWith("lens-rust");
		await rerender({
			lenses,
			activeLensId: "lens-rust",
			onSelect,
			onCreateClick: vi.fn(),
			onDelete,
		});

		await expect
			.element(page.getByText("Active lens: Rust"))
			.toBeInTheDocument();
		await expect.element(page.getByText("Delete?")).not.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /Confirm/ }))
			.not.toBeInTheDocument();
	});
});
