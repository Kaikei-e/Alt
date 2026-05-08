import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_ENTRY_KEY } from "../../infra/data/knowledge-loop";
import {
	field,
	installTransitionCapture,
} from "../../utils/transition-capture";

/**
 * Knowledge Loop Phase 2 — Decide / Act semantic feedback loop.
 *
 * The Decide workspace decision-option button must fire `/loop/transition`
 * with the *option's* semantic intent + target, not just a stage advance.
 * This is the regression guard for the case where workspace buttons were
 * calling `advanceEntry(activeEntry)` directly and dropping every signal of
 * which option the user actually picked.
 *
 * Reference: docs/plan/knowledge-loop-completion-02-semantic-decide-act.md
 */

test.describe("Knowledge Loop — Decide workspace fires semantic transition", () => {
	test("clicking the Revisit decision option sends acted_intent=revisit + target_type=entry", async ({
		page,
	}) => {
		const capture = await installTransitionCapture(page, {
			canonicalEntryKey: LOOP_FIXTURE_ENTRY_KEY,
		});

		await page.goto("/loop?lens=e2e-decide");

		// Pipeline: jump to Decide so the workspace renders the decision option
		// list (the Now fixture has decisionOptions revisit/ask/snooze).
		const pipeline = page.locator('[data-testid="loop-ooda-pipeline"]');
		await expect(pipeline).toBeVisible();
		const decideStage = pipeline.getByRole("button", { name: /decide/i });
		await decideStage.click();

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();
		await expect(workspace).toHaveAttribute("data-stage", "decide");

		const revisitOption = workspace.locator(
			'.decision-btn[data-intent="revisit"]',
		);
		await expect(revisitOption).toBeVisible();
		await revisitOption.click();

		await expect.poll(() => capture.posts.length).toBeGreaterThanOrEqual(1);
		const post = capture.posts[0];
		expect(field<string>(post, "entryKey", "entry_key")).toBe(
			LOOP_FIXTURE_ENTRY_KEY,
		);
		expect(field<string>(post, "actedIntent", "acted_intent")).toBe("revisit");
		expect(field<string>(post, "targetType", "target_type")).toBe("entry");
		expect(field<string>(post, "targetRef", "target_ref")).toBe(
			LOOP_FIXTURE_ENTRY_KEY,
		);
		expect(field<boolean>(post, "continueFlag", "continue_flag")).toBe(true);
		expect(
			field<string[]>(post, "presentedIntents", "presented_intents"),
		).toEqual(expect.arrayContaining(["revisit", "ask", "snooze"]));
	});
});
