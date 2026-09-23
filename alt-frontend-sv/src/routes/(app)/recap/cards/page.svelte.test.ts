import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	buildMockDegradedRecapCardsResponse,
	buildMockEmptyRecapCardsResponse,
	buildMockRecapCardsResponse,
} from "../../../../../tests/e2e/fixtures/factories/recapCardsFactory";
import TopicCardsPage from "./+page.svelte";

const { mockGetThreeDayRecapCards } = vi.hoisted(() => ({
	mockGetThreeDayRecapCards: vi.fn(),
}));

vi.mock("$lib/connect", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/connect")>();
	return {
		...actual,
		createClientTransport: vi.fn(() => ({})),
		getThreeDayRecapCards: mockGetThreeDayRecapCards,
	};
});

describe("Topic Cards Page (+page.svelte)", () => {
	it("renders loading skeleton initially", async () => {
		mockGetThreeDayRecapCards.mockImplementation(
			() => new Promise(() => {}), // Never resolves
		);

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-skeleton"))
			.toBeInTheDocument();
	});

	it("renders empty state when job is null", async () => {
		mockGetThreeDayRecapCards.mockResolvedValueOnce(
			buildMockEmptyRecapCardsResponse(),
		);

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
					"Three-day topic recap cards will appear here once generated.",
				),
			)
			.toBeInTheDocument();
	});

	it("renders degraded notice when job.degraded is true", async () => {
		mockGetThreeDayRecapCards.mockResolvedValueOnce(
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
		mockGetThreeDayRecapCards.mockRejectedValueOnce(
			new Error("BFF Connection Failed"),
		);

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

	it("renders populated topic cards in rank order", async () => {
		mockGetThreeDayRecapCards.mockResolvedValueOnce(
			buildMockRecapCardsResponse(),
		);

		render(TopicCardsPage);

		await expect
			.element(page.getByTestId("recap-cards-window"))
			.toBeInTheDocument();
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
