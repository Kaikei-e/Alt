import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type {
	BranchData,
	EpisodeData,
	FootprintData,
} from "$lib/connect/knowledge_trail";
import EpisodeCard from "./EpisodeCard.svelte";

function makeFootprint(overrides: Partial<FootprintData> = {}): FootprintData {
	return {
		footprintKey: "open:article:submarines:1",
		verb: "read",
		itemKey: "article:submarines",
		title: "Hunting Submarines Via Gravity",
		excerpt: "",
		tags: ["physics", "sensors"],
		note: "",
		occurredAt: "2026-07-03T09:00:00Z",
		firstOccurredAt: "2026-07-03T09:00:00Z",
		wear: "worn",
		contactCount: 1,
		...overrides,
	};
}

function makeEpisode(overrides: Partial<EpisodeData> = {}): EpisodeData {
	return {
		episodeKey: "episode:article:submarines",
		wear: "deep",
		thumbnailUrl: "",
		footprints: [makeFootprint()],
		...overrides,
	};
}

describe("EpisodeCard", () => {
	it("is collapsed by default: no member footprint rows are rendered", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							footprintKey: "asked:article:submarines:2",
							verb: "asked",
							occurredAt: "2026-07-05T09:00:00Z",
							firstOccurredAt: "2026-07-05T09:00:00Z",
						}),
						makeFootprint(),
					],
				}),
			},
		});
		await expect.element(page.getByTestId("trail-episode")).toBeInTheDocument();
		expect(page.getByTestId("trail-footprint").elements()).toHaveLength(0);
		await expect
			.element(page.getByTestId("episode-toggle"))
			.toHaveAttribute("aria-expanded", "false");
	});

	it("expanding the toggle reveals the member footprint rows", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							footprintKey: "asked:article:submarines:2",
							verb: "asked",
							occurredAt: "2026-07-05T09:00:00Z",
							firstOccurredAt: "2026-07-05T09:00:00Z",
						}),
						makeFootprint(),
					],
				}),
			},
		});
		const toggle = page.getByTestId("episode-toggle");
		await toggle.click();
		await expect.element(toggle).toHaveAttribute("aria-expanded", "true");
		expect(page.getByTestId("trail-footprint").elements()).toHaveLength(2);
	});

	it("renders a date range spanning the earliest and latest collapsed contact", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							occurredAt: "2026-07-07T09:00:00Z",
							firstOccurredAt: "2026-06-27T09:00:00Z",
							contactCount: 2,
						}),
					],
				}),
			},
		});
		const dates = page.getByTestId("episode-dates");
		await expect.element(dates).toHaveTextContent("Jun 27");
		await expect.element(dates).toHaveTextContent("Jul 7");
	});

	it("renders a single date when every contact falls on the same day", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							occurredAt: "2026-06-15T09:00:00Z",
							firstOccurredAt: "2026-06-15T09:00:00Z",
							contactCount: 1,
						}),
					],
				}),
			},
		});
		const dates = page.getByTestId("episode-dates");
		await expect.element(dates).toHaveTextContent("Jun 15");
		await expect.element(dates).not.toHaveTextContent("–");
	});

	it("summarizes contacts across members, summed per verb", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							footprintKey: "asked:article:submarines:2",
							verb: "asked",
							occurredAt: "2026-07-05T09:00:00Z",
							firstOccurredAt: "2026-07-05T09:00:00Z",
							contactCount: 1,
						}),
						makeFootprint({ contactCount: 1 }),
					],
				}),
			},
		});
		const summary = page.getByTestId("episode-contact");
		await expect.element(summary).toHaveTextContent("Read 1 time");
		await expect.element(summary).toHaveTextContent("asked 1 question");
	});

	it("sums contactCount into a single verb segment when all members share a verb", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					wear: "worn",
					footprints: [makeFootprint({ contactCount: 2 })],
				}),
			},
		});
		const summary = page.getByTestId("episode-contact");
		await expect.element(summary).toHaveTextContent("Read 2 times");
	});

	it("renders no thumbnail image when thumbnailUrl is empty", async () => {
		render(EpisodeCard, {
			props: { episode: makeEpisode({ thumbnailUrl: "" }) },
		});
		expect(page.getByTestId("episode-thumbnail").elements()).toHaveLength(0);
	});

	it("renders the thumbnail image when thumbnailUrl is present", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					thumbnailUrl: "https://cdn.example.com/thumb.jpg",
				}),
			},
		});
		await expect
			.element(page.getByTestId("episode-thumbnail"))
			.toBeInTheDocument();
	});

	it("links the representative title to the in-app article reader by id", async () => {
		render(EpisodeCard, { props: { episode: makeEpisode() } });
		const link = page.getByTestId("episode-link");
		await expect.element(link).toHaveAttribute("href", "/articles/submarines");
	});

	it("shows up to 3 tags from the representative footprint", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode({
					footprints: [
						makeFootprint({
							tags: ["physics", "sensors", "submarine-detection", "extra"],
						}),
					],
				}),
			},
		});
		expect(page.getByTestId("episode-tag").elements()).toHaveLength(3);
	});

	// Wave 9: trail search (D25) highlights matched members inside their
	// containing episode and auto-expands so the hit is visible without an
	// extra click.
	it("auto-expands when a member footprint's itemKey is in matchedItemKeys", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode(),
				matchedItemKeys: ["article:submarines"],
			},
		});
		await expect
			.element(page.getByTestId("episode-toggle"))
			.toHaveAttribute("aria-expanded", "true");
		// The sole member is itself the hit, so it renders under the hit
		// testid rather than the plain footprint one.
		expect(page.getByTestId("footprint-hit").elements()).toHaveLength(1);
	});

	it("marks the matched member footprint row with the hit testid", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode(),
				matchedItemKeys: ["article:submarines"],
			},
		});
		await expect.element(page.getByTestId("footprint-hit")).toBeInTheDocument();
	});

	it("does not auto-expand or mark hits when matchedItemKeys is empty (default)", async () => {
		render(EpisodeCard, { props: { episode: makeEpisode() } });
		await expect
			.element(page.getByTestId("episode-toggle"))
			.toHaveAttribute("aria-expanded", "false");
		expect(page.getByTestId("footprint-hit").elements()).toHaveLength(0);
	});

	it("does not auto-expand when matchedItemKeys names an item outside this episode", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode(),
				matchedItemKeys: ["article:something-else"],
			},
		});
		await expect
			.element(page.getByTestId("episode-toggle"))
			.toHaveAttribute("aria-expanded", "false");
	});
});

// Wave 10 (D26/D28): the top-of-trail branch inbox is removed. At most one
// branch surfaces per episode, subordinate to its header.
describe("EpisodeCard subordinate branch", () => {
	function makeBranch(overrides: Partial<BranchData> = {}): BranchData {
		return {
			branchKey: "cluster:u:article:z",
			anchorItemKey: "article:submarines",
			relationKind: "cluster",
			why: "Joins a topic you follow.",
			evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
			confidence: "plausible",
			targetItemKey: "article:z",
			targetTitle: "Async Rust",
			...overrides,
		};
	}

	it("renders no subordinate branch block when branch is absent", async () => {
		render(EpisodeCard, { props: { episode: makeEpisode() } });
		expect(page.getByTestId("episode-branch").elements()).toHaveLength(0);
	});

	it("renders one subordinate branch labeled 'Next step on this trail'", async () => {
		render(EpisodeCard, {
			props: {
				episode: makeEpisode(),
				branch: makeBranch(),
				onResolveBranch: vi.fn(),
			},
		});
		await expect
			.element(page.getByTestId("episode-branch"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("episode-branch"))
			.toHaveTextContent("Next step on this trail");
	});

	it("Take this path on the subordinate branch calls onResolveBranch as taken", async () => {
		const onResolveBranch = vi.fn();
		render(EpisodeCard, {
			props: { episode: makeEpisode(), branch: makeBranch(), onResolveBranch },
		});
		await page.getByTestId("branch-take").click();
		expect(onResolveBranch).toHaveBeenCalledWith(
			"cluster:u:article:z",
			"taken",
			"article:z",
		);
	});

	it("Dismiss on the subordinate branch opens the one-tap reason row", async () => {
		const onResolveBranch = vi.fn();
		render(EpisodeCard, {
			props: { episode: makeEpisode(), branch: makeBranch(), onResolveBranch },
		});
		await page.getByTestId("branch-dismiss").click();
		expect(onResolveBranch).not.toHaveBeenCalled();
		await page.getByTestId("branch-dismiss-reason-wrong_relation").click();
		expect(onResolveBranch).toHaveBeenCalledWith(
			"cluster:u:article:z",
			"dismissed",
			undefined,
			"wrong_relation",
		);
	});
});
