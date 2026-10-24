import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: vi.fn(() => ({ transport: true })),
}));

vi.mock("#lib/connect/knowledge_trail.js", () => ({
	getTrail: vi.fn(),
	searchTrail: vi.fn(),
	resolveBranch: vi.fn(),
}));

import { getTrail, searchTrail } from "#lib/connect/knowledge_trail.js";
import Page from "./+page.svelte";

describe("Knowledge Trail +page.svelte", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("matches at most one branch per episode by anchorItemKey membership and hides unmatched branches (D26/D28)", async () => {
		vi.mocked(getTrail).mockResolvedValueOnce({
			footprints: [],
			episodes: [
				{
					episodeKey: "ep-1",
					wear: "worn",
					thumbnailUrl: "",
					footprints: [
						{
							footprintKey: "fp-1",
							verb: "read",
							itemKey: "article:a1",
							title: "First Episode Article",
							excerpt: "",
							tags: ["systems"],
							note: "",
							occurredAt: "2026-03-20T12:00:00Z",
							wear: "worn",
							contactCount: 1,
							firstOccurredAt: "2026-03-20T12:00:00Z",
						},
						{
							footprintKey: "fp-2",
							verb: "read",
							itemKey: "article:a2",
							title: "Earlier Member",
							excerpt: "",
							tags: ["systems"],
							note: "",
							occurredAt: "2026-03-19T12:00:00Z",
							wear: "thin",
							contactCount: 1,
							firstOccurredAt: "2026-03-19T12:00:00Z",
						},
					],
				},
				{
					episodeKey: "ep-2",
					wear: "thin",
					thumbnailUrl: "",
					footprints: [
						{
							footprintKey: "fp-3",
							verb: "read",
							itemKey: "article:b1",
							title: "Second Episode Without Branch",
							excerpt: "",
							tags: [],
							note: "",
							occurredAt: "2026-03-18T12:00:00Z",
							wear: "thin",
							contactCount: 1,
							firstOccurredAt: "2026-03-18T12:00:00Z",
						},
					],
				},
			],
			branches: [
				{
					branchKey: "br-first",
					anchorItemKey: "article:a1",
					relationKind: "continuation",
					why: "Primary continuation for episode 1",
					evidenceRefs: [{ refId: "e1", label: "Shared tag", kind: "tag" }],
					confidence: "high",
					targetItemKey: "article:target-1",
					targetTitle: "First Matched Branch Target",
				},
				{
					branchKey: "br-second-same-ep",
					anchorItemKey: "article:a2",
					relationKind: "cluster",
					why: "Secondary branch on same episode should be suppressed",
					evidenceRefs: [],
					confidence: "medium",
					targetItemKey: "article:target-2",
					targetTitle: "Suppressed Second Branch Target",
				},
				{
					branchKey: "br-orphan",
					anchorItemKey: "article:unmatched",
					relationKind: "contradiction",
					why: "Orphan branch not anchored in any shown episode",
					evidenceRefs: [],
					confidence: "low",
					targetItemKey: "article:target-3",
					targetTitle: "Orphan Unmatched Branch Target",
				},
			],
			nextCursor: "",
			hasMore: false,
		});

		render(Page);

		await expect
			.element(page.getByText("First Episode Article").first())
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Second Episode Without Branch"))
			.toBeInTheDocument();

		// Only the first matched branch for ep-1 is rendered
		await expect
			.element(page.getByText("First Matched Branch Target"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Suppressed Second Branch Target"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText("Orphan Unmatched Branch Target"))
			.not.toBeInTheDocument();
	});

	it("renders trail.searchError in an alert banner when trail search fails", async () => {
		vi.mocked(getTrail).mockResolvedValueOnce({
			footprints: [],
			episodes: [],
			branches: [],
			nextCursor: "",
			hasMore: false,
		});
		vi.mocked(searchTrail).mockRejectedValueOnce(
			new Error("Trail search failed: backend timeout"),
		);

		render(Page);

		const input = page.getByTestId("trail-search");
		await input.fill("consensus protocols");

		const submitBtn = page.getByRole("button", { name: "Search trail" });
		await submitBtn.click();

		const errorAlert = page.getByTestId("trail-search-error");
		await expect.element(errorAlert).toBeInTheDocument();
		await expect.element(errorAlert).toHaveAttribute("role", "alert");
		await expect
			.element(errorAlert)
			.toHaveTextContent("Trail search failed: backend timeout");
	});
});
