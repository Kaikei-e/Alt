import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

vi.mock("#lib/connect/reading-transport.js", async (importOriginal) => ({
	...(await importOriginal<
		typeof import("#lib/connect/reading-transport.js")
	>()),
	getReadingTransport: (keepalive = false) =>
		keepalive ? "keepalive-transport" : "client-transport",
}));

vi.mock("#lib/connect/reading.js", async (importOriginal) => {
	const actual =
		await importOriginal<typeof import("#lib/connect/reading.js")>();
	return {
		...actual,
		listThemes: vi.fn(),
	};
});

import { listThemes } from "#lib/connect/reading.js";
import { formatCompactDate } from "#lib/reading/themes-format.js";
import Page from "./+page.svelte";

async function settle(ms = 80) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/knowledge Themes list page", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders loading state initially", async () => {
		vi.mocked(listThemes).mockImplementation(() => new Promise(() => {}));

		render(Page);

		await expect
			.element(page.getByTestId("themes-loading"))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("heading", { level: 1, name: "Themes" }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Pages built only from articles you have read."))
			.toBeInTheDocument();
	});

	it("renders theme rows with links, meta line, status suffixes, and Knowledge Trail link", async () => {
		const date1 = new Date("2026-11-18T10:00:00.000Z");
		const date2 = new Date("2026-11-17T08:00:00.000Z");
		const date3 = new Date("2026-11-16T08:00:00.000Z");

		vi.mocked(listThemes).mockResolvedValueOnce([
			{
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				title: "Compiler Toolchains",
				latestVersion: 2,
				articleCount: 3,
				updatedAt: date1,
				lastOpenedAt: null,
				lastOpenedVersion: 0,
			},
			{
				themeId: "01938e82-7c00-7a7b-9b10-100000000002",
				title: "Query Planners",
				latestVersion: 3,
				articleCount: 5,
				updatedAt: date2,
				lastOpenedAt: new Date("2026-11-16T12:00:00.000Z"),
				lastOpenedVersion: 1,
			},
			{
				themeId: "01938e82-7c00-7a7b-9b10-100000000003",
				title: "Storage Engines",
				latestVersion: 1,
				articleCount: 2,
				updatedAt: date3,
				lastOpenedAt: new Date("2026-11-16T09:00:00.000Z"),
				lastOpenedVersion: 1,
			},
		]);

		render(Page);
		await settle();

		const rows = page.getByTestId("theme-row");
		expect(rows.elements()).toHaveLength(3);

		const firstLink = page.getByRole("link", { name: /Compiler Toolchains/i });
		await expect
			.element(firstLink)
			.toHaveAttribute(
				"href",
				"/knowledge/themes/01938e82-7c00-7a7b-9b10-100000000001",
			);
		await expect
			.element(firstLink)
			.toHaveTextContent(
				`3 articles · updated ${formatCompactDate(date1)} · new`,
			);

		const secondLink = page.getByRole("link", { name: /Query Planners/i });
		await expect
			.element(secondLink)
			.toHaveAttribute(
				"href",
				"/knowledge/themes/01938e82-7c00-7a7b-9b10-100000000002",
			);
		await expect
			.element(secondLink)
			.toHaveTextContent(
				`5 articles · updated ${formatCompactDate(date2)} · updated since you read it`,
			);

		const thirdLink = page.getByRole("link", { name: /Storage Engines/i });
		await expect
			.element(thirdLink)
			.toHaveTextContent(`2 articles · updated ${formatCompactDate(date3)}`);
		await expect.element(thirdLink).not.toHaveTextContent("· new");
		await expect
			.element(thirdLink)
			.not.toHaveTextContent("updated since you read it");

		await expect
			.element(page.getByRole("link", { name: "Knowledge Trail" }))
			.not.toBeInTheDocument();
	});

	it("renders empty state when no themes exist", async () => {
		vi.mocked(listThemes).mockResolvedValueOnce([]);

		render(Page);
		await settle();

		expect(listThemes).toHaveBeenCalledWith("client-transport");
		await expect.element(page.getByTestId("themes-empty")).toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					"No themes yet. A theme appears once you have read a few articles on the same subject.",
				),
			)
			.toBeInTheDocument();
	});

	it("renders error state and retries when Retry is clicked", async () => {
		vi.mocked(listThemes)
			.mockRejectedValueOnce(new Error("Service unavailable"))
			.mockResolvedValueOnce([
				{
					themeId: "01938e82-7c00-7a7b-9b10-100000000001",
					title: "Recovered Theme",
					latestVersion: 1,
					articleCount: 2,
					updatedAt: new Date("2026-11-18T10:00:00.000Z"),
					lastOpenedAt: null,
					lastOpenedVersion: 0,
				},
			]);

		render(Page);
		await settle();

		await expect.element(page.getByTestId("themes-error")).toBeInTheDocument();
		const retryBtn = page.getByRole("button", { name: "Retry" });
		await retryBtn.click();
		await settle();

		await expect
			.element(page.getByRole("link", { name: /Recovered Theme/i }))
			.toBeInTheDocument();
	});
});
