import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../../../../tests/e2e/fixtures/factories/recapCardsFactory";
import TopicCardsPage from "./+page.svelte";

const { mockGetTopicCards } = vi.hoisted(() => ({
	mockGetTopicCards: vi.fn(),
}));

vi.mock("$lib/connect", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/connect")>();
	return {
		...actual,
		createClientTransport: vi.fn(() => ({})),
		getTopicCards: mockGetTopicCards,
	};
});

describe("Topic Cards Page (+page.svelte)", () => {
	it("renders loading skeleton initially", async () => {
		mockGetTopicCards.mockImplementation(
			() => new Promise(() => {}), // Never resolves
		);

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-skeleton"))
			.toBeInTheDocument();
	});

	it("renders empty state when job is null and no active run", async () => {
		mockGetTopicCards.mockResolvedValueOnce(buildMockEmptyRecapCardsResponse());

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-empty"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("No topic cards yet"))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					"Topic cards will appear here after the first daily run.",
				),
			)
			.toBeInTheDocument();
	});

	it("renders empty state with 'No topic cards were selected in the latest run.' when job exists but cards array is empty", async () => {
		mockGetTopicCards.mockResolvedValueOnce({
			job: {
				jobId: "job-no-cards",
				kickedAt: "2026-09-22T17:00:00Z",
				from: "2026-09-19T17:00:00Z",
				to: "2026-09-22T17:00:00Z",
				paramsVersion: "cards-v0.2",
				cardsSelected: 0,
				degraded: false,
			},
			cards: [],
		});

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-empty"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("No topic cards yet"))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText("No topic cards were selected in the latest run."),
			)
			.toBeInTheDocument();
	});

	it("renders running status notice when latestRun is running", async () => {
		const mockData = {
			...buildMockRecapCardsResponse(),
			latestRun: {
				jobId: "run-running",
				status: "running",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:01:00Z",
			},
		};
		mockGetTopicCards.mockResolvedValueOnce(mockData);

		render(TopicCardsPage);

		await expect
			.element(page.getByText(/Update in progress \(started /))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("status"))
			.toHaveTextContent("Update in progress");
	});

	it("renders failed status notice when latestRun is failed", async () => {
		const mockData = {
			...buildMockRecapCardsResponse(),
			latestRun: {
				jobId: "run-failed",
				status: "failed",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:02:00Z",
			},
		};
		mockGetTopicCards.mockResolvedValueOnce(mockData);

		render(TopicCardsPage);

		await expect
			.element(
				page.getByText(/The latest update failed at .* Showing cards from /),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("alert"))
			.toHaveTextContent("The latest update failed");
	});

	it("renders status banner and does not render empty state when job is null and latestRun is pending", async () => {
		mockGetTopicCards.mockResolvedValueOnce({
			job: null,
			cards: [],
			latestRun: {
				jobId: "run-pending-no-job",
				status: "pending",
				kickedAt: "2026-09-22T19:00:00Z",
				updatedAt: "2026-09-22T19:00:10Z",
			},
		});

		render(TopicCardsPage);

		await expect
			.element(page.getByRole("status"))
			.toHaveTextContent("Update in progress");
		await expect
			.element(page.getByTestId("recap-cards-empty"))
			.not.toBeInTheDocument();
	});

	it("renders degraded notice when job.degraded is true", async () => {
		mockGetTopicCards.mockResolvedValueOnce(
			buildMockDegradedRecapCardsResponse(),
		);

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-degraded"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/Degraded generation/i))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("recap-topic-card"))
			.toBeInTheDocument();
	});

	it("renders error state when fetch fails", async () => {
		mockGetTopicCards.mockRejectedValueOnce(new Error("BFF Connection Failed"));

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-error"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Topic cards could not be loaded. Try again."))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("BFF Connection Failed"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole("button", { name: /retry/i }))
			.toBeInTheDocument();
	});

	it("renders populated topic cards in rank order with freshness line", async () => {
		mockGetTopicCards.mockResolvedValueOnce(buildMockRecapCardsResponse());

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-window"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("recap-cards-freshness"))
			.toBeInTheDocument();
		await expect.element(page.getByText(/Updated /)).toBeInTheDocument();
		await expect
			.element(page.getByText("大規模推論モデルの国内展開が加速"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("次世代Webレンダリング標準の策定議論"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("分散データベースの自動修復機能検証"))
			.toBeInTheDocument();
	});
});
