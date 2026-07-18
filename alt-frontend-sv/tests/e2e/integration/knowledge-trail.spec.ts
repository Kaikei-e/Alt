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

		const spine = page.getByTestId("trail-spine");
		await expect(spine).toBeVisible({ timeout: 15000 });

		const footprints = page.getByTestId("trail-footprint");
		const empty = page.getByTestId("trail-empty");
		await expect(footprints.first().or(empty)).toBeVisible({ timeout: 15000 });
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

const TRAIL_WITH_BRANCH = {
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
};

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
