import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import RecapPreviewModal from "./RecapPreviewModal.svelte";
import type { RecapModalData } from "./types";

const fullData: RecapModalData = {
	genre: "Technology",
	summary: "AI advances including LLM breakthroughs",
	topTerms: ["LLM", "agents", "RAG"],
	windowDays: 3,
	executedAt: "2026-04-08T12:00:00Z",
	bullets: ["Models improved significantly", "New architectures emerged"],
	tags: ["ai", "tech"],
	jobId: "job-abc",
};

function renderModal(overrides: Partial<RecapModalData> | null = {}) {
	const data = overrides === null ? null : { ...fullData, ...overrides };
	return render(RecapPreviewModal, {
		props: {
			data,
			open: true,
			onOpenChange: vi.fn(),
		},
	});
}

describe("RecapPreviewModal", () => {
	it("renders genre title when open", async () => {
		renderModal();
		await expect.element(page.getByText("Technology")).toBeInTheDocument();
	});

	it("renders summary text", async () => {
		renderModal();
		await expect
			.element(page.getByText("AI advances including LLM breakthroughs"))
			.toBeInTheDocument();
	});

	it("renders bullet points when present", async () => {
		renderModal();
		await expect
			.element(page.getByText("Models improved significantly"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("New architectures emerged"))
			.toBeInTheDocument();
	});

	it("does not render Key Points section when bullets is undefined", async () => {
		renderModal({ bullets: undefined });
		await expect.element(page.getByText("Key Points")).not.toBeInTheDocument();
	});

	it("does not render Key Points section when bullets is empty", async () => {
		renderModal({ bullets: [] });
		await expect.element(page.getByText("Key Points")).not.toBeInTheDocument();
	});

	// `exact: true` is load-bearing here, not decoration: the default text
	// locator is a case-insensitive substring match, so plain getByText("LLM")
	// also matches the summary paragraph ("AI advances including LLM
	// breakthroughs") and getByText("tech") also matches the "Technology"
	// title, which is a strict-mode violation rather than a passing assertion.
	// Exact matching is the stricter locator: it can only resolve to the badge.
	it("renders top terms as badges", async () => {
		renderModal();
		await expect
			.element(page.getByText("LLM", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("agents", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("RAG", { exact: true }))
			.toBeInTheDocument();
	});

	it("renders tags when present", async () => {
		renderModal();
		await expect
			.element(page.getByText("ai", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("tech", { exact: true }))
			.toBeInTheDocument();
	});

	it("does not render Tags section when tags is undefined", async () => {
		renderModal({ tags: undefined });
		await expect.element(page.getByText("Tags")).not.toBeInTheDocument();
	});

	it("renders window days badge", async () => {
		renderModal();
		await expect.element(page.getByText("3-day")).toBeInTheDocument();
	});

	it("does not render content when data is null", async () => {
		renderModal(null);
		await expect.element(page.getByText("Technology")).not.toBeInTheDocument();
	});
});
