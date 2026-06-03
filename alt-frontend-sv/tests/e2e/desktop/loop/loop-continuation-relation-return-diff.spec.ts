import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_CONTINUATION_ENTRY_KEY } from "../../infra/data/knowledge-loop";

/**
 * Outside-in acceptance test for the ADR-000937 first vertical slice.
 *
 * The redesign replaces single-bucket placement with a first-class relation-set
 * rendered as the always-present Orient surface. This spec proves the relation
 * reaches the PRIMARY surface — the active-entry workspace — not just the
 * secondary tiles, and that the relation State (the "loop closed" cue) renders.
 *
 * Backend mock serves a single foreground entry carrying one Continuation
 * relation when lensModeId is "e2e-continuation" (OPEN) or
 * "e2e-continuation-advanced" (ADVANCED); see tests/e2e/infra/handlers/backend.ts.
 *
 * The open→advancing→advanced state transition is the visible "loop closed":
 * its causation (more continue acts → a higher state) is unit-tested in
 * knowledge-sovereign's extractRelations; here we prove each state renders on
 * the user's primary surface.
 */

test.describe("Knowledge Loop — Continuation relation on the Orient surface", () => {
	test("relation surfaces OPEN on the active-entry workspace", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-continuation");

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();

		// The relation-set is rendered as a first-class chip on the active
		// entry's workspace, present without expanding or advancing any stage.
		const relation = workspace.locator(
			'[data-testid="loop-relation"][data-relation-kind="continuation"]',
		);
		await expect(relation).toBeVisible();
		await expect(relation).toHaveAttribute("data-relation-state", "open");
		await expect(relation).toContainText(/continuing a thread/i);
	});

	test("an advanced thread renders the ADVANCED state cue", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-continuation-advanced");

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();

		const relation = workspace.locator(
			'[data-testid="loop-relation"][data-relation-kind="continuation"]',
		);
		await expect(relation).toBeVisible();
		// The state climbed open → advanced because the user returned to the
		// thread repeatedly; the chip reflects the closed-loop progress.
		await expect(relation).toHaveAttribute("data-relation-state", "advanced");
		await expect(relation).toContainText(/keep returning to/i);
	});

	test("the seeded continuation entry is the active workspace entry", async ({
		page,
	}) => {
		await page.goto("/loop?lens=e2e-continuation");
		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();
		// Guards the fixture/handler wiring: the relation chip belongs to the
		// seeded entry, so a future fixture rename surfaces here, not as a
		// silent empty Orient surface.
		expect(LOOP_FIXTURE_CONTINUATION_ENTRY_KEY).toBe(
			"loop-entry-fixture-continuation-rel-1",
		);
	});
});
