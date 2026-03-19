import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { LensData } from "$lib/connect/knowledge_home";
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
		render(LensSelector as never, {
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
		render(LensSelector as never, {
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
		render(LensSelector as never, {
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
		render(LensSelector as never, {
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
		render(LensSelector as never, {
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
		render(LensSelector as never, {
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
});
