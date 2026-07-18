import { expect, test } from "@playwright/test";
import { fulfillJson } from "../utils/mockHelpers";

// E2E for the Knowledge Trail spine (Wave 2, read-only). Pull-only: the page
// loads on navigation and refreshes only on explicit action — there is no live
// channel to wait on (the deliberate PM-2026-039 / PM-2026-045 lesson).
test.describe("Knowledge Trail spine", () => {
	test("loads the trail page with an editorial header", async ({ page }) => {
		await page.goto("./knowledge/trail");
		await expect(page.getByRole("heading", { name: "Your Trail" })).toBeVisible(
			{
				timeout: 15000,
			},
		);
	});

	test("renders either footprints or the empty-state, never a spinner forever", async ({
		page,
	}) => {
		await page.goto("./knowledge/trail");

		// The spine renders episodes (Wave 8) — either at least one episode card
		// or the explicit empty-state, never an eternal spinner.
		const episodes = page.getByTestId("trail-episode");
		const empty = page.getByTestId("trail-empty");
		await expect(episodes.first().or(empty)).toBeVisible({ timeout: 15000 });
	});

	test("exposes an explicit refresh affordance (pull-only)", async ({
		page,
	}) => {
		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-refresh")).toBeVisible({
			timeout: 15000,
		});
	});
});

const TRAIL_PATHS = {
	getTrail: "**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
	resolveBranch:
		"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/ResolveBranch",
	emitTrailOutcome:
		"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/EmitTrailOutcome",
};

const BRANCH_KEY = "cluster:u1:article:b2";

// `footprints` is now the legacy flat field (empty once the episode spine
// ships); the spine's default display unit is `episodes` (Wave 8, D24).
const TRAIL_WITH_BRANCH = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: "2026-07-18T00:00:00Z",
	branches: [
		{
			branchKey: BRANCH_KEY,
			anchorItemKey: "article:a1",
			relationKind: "cluster",
			why: "Joins a topic you follow.",
			evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
			confidence: "plausible",
			targetItemKey: "article:b2",
			targetTitle: "Async Rust",
		},
	],
	episodes: [
		{
			episodeKey: "episode:article:a1",
			wear: "thin",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:a1",
					verb: "read",
					itemKey: "article:a1",
					title: "io_uring basics",
					excerpt: "",
					tags: ["rust"],
					note: "",
					occurredAt: "2026-07-17T09:00:00Z",
					wear: "thin",
				},
			],
		},
	],
};

// Wave 7 noise removal: the lens chip bar (a raw tag union — the dead tag
// cloud) is gone entirely, and repeated contacts with one article collapse
// server-side into a single spine row carrying a visit count instead of one
// row per day (D24/D25). Wave 8 folds this collapsed footprint into a single
// episode; the visit count now surfaces on the episode header.
const TRAIL_COLLAPSED = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: "2026-07-18T00:00:00Z",
	branches: [],
	episodes: [
		{
			episodeKey: "episode:article:a1",
			wear: "worn",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:a1",
					verb: "read",
					itemKey: "article:a1",
					title: "US military courts in the UK",
					excerpt: "",
					tags: ["military", "british-courts"],
					note: "",
					occurredAt: "2026-07-07T22:20:00Z",
					firstOccurredAt: "2026-06-27T18:37:00Z",
					contactCount: 2,
					wear: "worn",
				},
			],
		},
	],
};

test.describe("Trail noise removal", () => {
	test("does not render a lens chip bar even when footprints carry tags", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
			(route) => fulfillJson(route, TRAIL_COLLAPSED),
		);
		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode").first()).toBeVisible({
			timeout: 15000,
		});
		await expect(page.getByTestId("trail-lenses")).toHaveCount(0);
	});

	test("renders a collapsed footprint once, with its visit count on the episode header", async ({
		page,
	}) => {
		await page.route(
			"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
			(route) => fulfillJson(route, TRAIL_COLLAPSED),
		);
		await page.goto("./knowledge/trail");
		// D24 collapse: one episode, not one row per contact.
		await expect(page.getByTestId("trail-episode")).toHaveCount(1, {
			timeout: 15000,
		});
		await expect(page.getByTestId("episode-contact")).toContainText("2");
	});
});

// Wave 5 trail closure: taking a branch means walking it. The resolve emits,
// the user lands on the article, and leaving the article emits the raw dwell
// outcome (trail.act_outcome.v1 upstream). Dwell is measured only when the
// navigation carries ?trail_proposal= — organic reads never emit.
test.describe("Trail closure (dwell outcome)", () => {
	test("take-path resolves, walks to the article, and emits dwell on leave", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TRAIL_WITH_BRANCH),
		);
		await page.route(TRAIL_PATHS.resolveBranch, (route) =>
			fulfillJson(route, { ok: true }),
		);
		let outcomeBody: Record<string, unknown> | null = null;
		await page.route(TRAIL_PATHS.emitTrailOutcome, async (route) => {
			outcomeBody = route.request().postDataJSON();
			await fulfillJson(route, { ok: true });
		});

		await page.goto("./knowledge/trail");
		await page.getByTestId("branch-take").click();

		// Taking the branch walks to the article, carrying the proposal gate.
		await page.waitForURL(/\/articles\/b2\?/, { timeout: 15000 });
		expect(page.url()).toContain("trail_proposal=");

		// Leaving the article flushes the dwell outcome exactly once.
		await page.goBack();
		await expect.poll(() => outcomeBody, { timeout: 15000 }).not.toBeNull();
		const body = outcomeBody as unknown as Record<string, unknown>;
		expect(body.branchKey).toBe(BRANCH_KEY);
		expect(body.itemKey).toBe("article:b2");
		// int64 serializes as a JSON string in Connect JSON; only the magnitude matters.
		expect(Number(body.dwellMs)).toBeGreaterThanOrEqual(0);
	});

	test("organic article visits never emit a trail outcome", async ({
		page,
	}) => {
		let emitted = false;
		await page.route(TRAIL_PATHS.emitTrailOutcome, async (route) => {
			emitted = true;
			await fulfillJson(route, { ok: true });
		});
		await page.goto("./articles/b2");
		await page.waitForTimeout(500);
		await page.goBack();
		await page.waitForTimeout(500);
		expect(emitted).toBe(false);
	});
});

// Wave 8: the spine's default display unit is the derived episode, not the
// raw footprint (D24/D30). Same-article contacts fold into one card; date is
// a landmark on the header, never a grouping axis — the old day-separator is
// gone entirely.
const EPISODE_TRAIL = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: "2026-07-18T00:00:00Z",
	branches: [],
	episodes: [
		{
			episodeKey: "episode:article:us-military",
			wear: "worn",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:us-military",
					verb: "read",
					itemKey: "article:us-military",
					title:
						"US military push for right to court-martial troops stationed in the UK",
					excerpt: "",
					tags: ["military", "uk-us-relations"],
					note: "",
					occurredAt: "2026-07-07T09:00:00Z",
					firstOccurredAt: "2026-06-27T09:00:00Z",
					contactCount: 2,
					wear: "worn",
				},
			],
		},
		{
			episodeKey: "episode:article:submarines",
			wear: "deep",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "asked:article:submarines:2",
					verb: "asked",
					itemKey: "article:submarines",
					title: "Hunting Submarines Via Gravity",
					excerpt: "",
					tags: ["physics", "sensors"],
					note: "",
					occurredAt: "2026-07-05T09:00:00Z",
					firstOccurredAt: "2026-07-05T09:00:00Z",
					contactCount: 1,
					wear: "deep",
				},
				{
					footprintKey: "open:article:submarines:1",
					verb: "read",
					itemKey: "article:submarines",
					title: "Hunting Submarines Via Gravity",
					excerpt: "",
					tags: ["physics", "sensors"],
					note: "",
					occurredAt: "2026-07-03T09:00:00Z",
					firstOccurredAt: "2026-07-03T09:00:00Z",
					contactCount: 1,
					wear: "worn",
				},
			],
		},
	],
};

test.describe("Episode spine", () => {
	test("renders episodes, collapsed by default, with no day-separator grouping", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, EPISODE_TRAIL),
		);
		await page.goto("./knowledge/trail");

		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		// Collapsed by default: member footprint rows are not rendered yet.
		await expect(page.getByTestId("trail-footprint")).toHaveCount(0);

		// D24: date is a landmark, never a grouping axis — the old day-separator
		// block (TrailSpine's `.day-sep`) must not exist at all.
		await expect(page.locator(".day-sep")).toHaveCount(0);
	});

	test("shows a date range and a contact summary on the episode header", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, EPISODE_TRAIL),
		);
		await page.goto("./knowledge/trail");

		const militaryEpisode = page
			.getByTestId("trail-episode")
			.filter({ hasText: "US military push" });
		await expect(militaryEpisode).toBeVisible({ timeout: 15000 });
		// Range spans the earliest and latest collapsed contact.
		await expect(militaryEpisode).toContainText("Jun 27");
		await expect(militaryEpisode).toContainText("Jul 7");
		// Contact summary sums contactCount per verb.
		await expect(militaryEpisode).toContainText("Read 2 times");
	});

	test("expanding an episode reveals its member footprint rows", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, EPISODE_TRAIL),
		);
		await page.goto("./knowledge/trail");

		const submarinesEpisode = page
			.getByTestId("trail-episode")
			.filter({ hasText: "Hunting Submarines Via Gravity" });
		await expect(submarinesEpisode).toBeVisible({ timeout: 15000 });
		await expect(submarinesEpisode).toContainText("Read 1 time");
		await expect(submarinesEpisode).toContainText("asked 1 question");

		const toggle = submarinesEpisode.getByTestId("episode-toggle");
		await expect(toggle).toHaveAttribute("aria-expanded", "false");
		await expect(submarinesEpisode.getByTestId("trail-footprint")).toHaveCount(
			0,
		);

		await toggle.click();

		await expect(toggle).toHaveAttribute("aria-expanded", "true");
		await expect(submarinesEpisode.getByTestId("trail-footprint")).toHaveCount(
			2,
		);
	});
});

// Wave 9: trail search is the sole rediscovery instrument (D25). Pull-only —
// fetch happens only on explicit submit (Enter or the search button), never
// from a keystroke or an $effect. One input, under the trail header.
const SEARCH_PATH =
	"**/api/v2/alt.knowledge_trail.v1.KnowledgeTrailService/SearchTrail";

const TWO_EPISODE_TRAIL = {
	footprints: [],
	nextCursor: "",
	hasMore: false,
	generatedAt: "2026-07-18T00:00:00Z",
	branches: [],
	episodes: [
		{
			episodeKey: "episode:article:submarines",
			wear: "deep",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:submarines:1",
					verb: "read",
					itemKey: "article:submarines",
					title: "Hunting Submarines Via Gravity",
					excerpt: "",
					tags: ["physics"],
					note: "",
					occurredAt: "2026-07-05T09:00:00Z",
					firstOccurredAt: "2026-07-03T09:00:00Z",
					contactCount: 2,
					wear: "deep",
				},
			],
		},
		{
			episodeKey: "episode:article:async",
			wear: "thin",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:async:1",
					verb: "read",
					itemKey: "article:async",
					title: "io_uring and the future of async I/O on Linux",
					excerpt: "",
					tags: ["rust"],
					note: "",
					occurredAt: "2026-06-20T09:00:00Z",
					firstOccurredAt: "2026-06-20T09:00:00Z",
					contactCount: 1,
					wear: "thin",
				},
			],
		},
	],
};

const SEARCH_HIT = {
	episodes: [
		{
			episodeKey: "episode:article:submarines",
			wear: "deep",
			thumbnailUrl: "",
			footprints: [
				{
					footprintKey: "open:article:submarines:1",
					verb: "read",
					itemKey: "article:submarines",
					title: "Hunting Submarines Via Gravity",
					excerpt: "",
					tags: ["physics"],
					note: "",
					occurredAt: "2026-07-05T09:00:00Z",
					firstOccurredAt: "2026-07-03T09:00:00Z",
					contactCount: 2,
					wear: "deep",
				},
			],
		},
	],
	matchedItemKeys: ["article:submarines"],
};

const SEARCH_EMPTY = { episodes: [], matchedItemKeys: [] };

test.describe("Trail search", () => {
	test("submitting a query with Enter renders only the matching episode and highlights the matched member", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TWO_EPISODE_TRAIL),
		);
		await page.route(SEARCH_PATH, (route) => fulfillJson(route, SEARCH_HIT));

		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search").fill("submarine");
		await page.getByTestId("trail-search").press("Enter");

		await expect(page.getByTestId("trail-episode")).toHaveCount(1, {
			timeout: 15000,
		});
		await expect(page.getByTestId("trail-episode")).toContainText(
			"Hunting Submarines Via Gravity",
		);
		await expect(page.getByTestId("footprint-hit")).toBeVisible();
	});

	test("the clear affordance restores the full spine", async ({ page }) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TWO_EPISODE_TRAIL),
		);
		await page.route(SEARCH_PATH, (route) => fulfillJson(route, SEARCH_HIT));

		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search").fill("submarine");
		await page.getByTestId("trail-search").press("Enter");
		await expect(page.getByTestId("trail-episode")).toHaveCount(1, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search-clear").click();
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});
	});

	test("a zero-hit query shows the explicit empty search state", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TWO_EPISODE_TRAIL),
		);
		await page.route(SEARCH_PATH, (route) => fulfillJson(route, SEARCH_EMPTY));

		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search").fill("nonexistent-xyz");
		await page.getByTestId("trail-search").press("Enter");

		await expect(page.getByTestId("trail-search-empty")).toBeVisible({
			timeout: 15000,
		});
		await expect(page.getByTestId("trail-episode")).toHaveCount(0);
	});

	test("typing without submitting never calls SearchTrail (pull-only)", async ({
		page,
	}) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TWO_EPISODE_TRAIL),
		);
		let searchCalled = false;
		await page.route(SEARCH_PATH, async (route) => {
			searchCalled = true;
			await fulfillJson(route, SEARCH_HIT);
		});

		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search").fill("submarine");
		await page.waitForTimeout(500);
		expect(searchCalled).toBe(false);
	});

	test("submitting an empty query is a no-op", async ({ page }) => {
		await page.route(TRAIL_PATHS.getTrail, (route) =>
			fulfillJson(route, TWO_EPISODE_TRAIL),
		);
		let searchCalled = false;
		await page.route(SEARCH_PATH, async (route) => {
			searchCalled = true;
			await fulfillJson(route, SEARCH_HIT);
		});

		await page.goto("./knowledge/trail");
		await expect(page.getByTestId("trail-episode")).toHaveCount(2, {
			timeout: 15000,
		});

		await page.getByTestId("trail-search").press("Enter");
		await page.waitForTimeout(300);
		expect(searchCalled).toBe(false);
		await expect(page.getByTestId("trail-episode")).toHaveCount(2);
	});
});
