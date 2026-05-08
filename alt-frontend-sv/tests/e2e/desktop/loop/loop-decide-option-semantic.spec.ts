import { expect, test } from "@playwright/test";
import { LOOP_FIXTURE_DECIDE_ENTRY_KEY } from "../../infra/data/knowledge-loop";
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
	test("clicking the Snooze decision option sends acted_intent=snooze + target_type=entry + continue_flag=false", async ({
		page,
	}) => {
		const capture = await installTransitionCapture(page, {
			canonicalEntryKey: LOOP_FIXTURE_DECIDE_ENTRY_KEY,
		});

		// The e2e-decide fixture pre-positions the entry at currentEntryStage=DECIDE
		// so the workspace renders the decision-option list directly. Snooze is
		// the cleanest semantic option to assert here: it routes through the
		// canonical `defer` same-stage transition (contract §8.2) and exercises
		// the Phase 2 metadata threading via `loop.dismiss(entryKey, metadata)`.
		await page.goto("/loop?lens=e2e-decide");

		const workspace = page.getByTestId("loop-ooda-workspace");
		await expect(workspace).toBeVisible();
		await expect(workspace).toHaveAttribute("data-stage", "decide");

		const snoozeOption = workspace.locator(
			'.decision-btn[data-intent="snooze"]',
		);
		await expect(snoozeOption).toBeVisible();
		await snoozeOption.click();

		await expect.poll(() => capture.posts.length).toBeGreaterThanOrEqual(1);
		const post = capture.posts[0];
		expect(field<string>(post, "entryKey", "entry_key")).toBe(
			LOOP_FIXTURE_DECIDE_ENTRY_KEY,
		);
		expect(field<string>(post, "actedIntent", "acted_intent")).toBe("snooze");
		expect(field<string>(post, "targetType", "target_type")).toBe("entry");
		expect(field<string>(post, "targetRef", "target_ref")).toBe(
			LOOP_FIXTURE_DECIDE_ENTRY_KEY,
		);
		expect(field<boolean>(post, "continueFlag", "continue_flag")).toBe(false);
		expect(field<string>(post, "trigger", "trigger")).toBe("defer");
		expect(
			field<string[]>(post, "presentedIntents", "presented_intents"),
		).toEqual(expect.arrayContaining(["revisit", "ask", "snooze"]));
	});
});
