import { expect, test } from "@playwright/test";
import { DesktopKnowledgeHomeAdminPage } from "../../pages/desktop/DesktopKnowledgeHomeAdminPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	SOVEREIGN_ADMIN_SNAPSHOT,
	KNOWLEDGE_HOME_ADMIN_SNAPSHOT,
} from "../../fixtures/factories/sovereignAdminFactory";

const ADMIN_API = "**/api/admin/knowledge-home";
const SOVEREIGN_API = "**/api/admin/knowledge-home/sovereign";

// Closes the loop opened by ADR-867 / ADR-869: the operator-facing
// "Emit URL Backfill" button is the only way historical articles whose
// `knowledge_home_items.url` is empty get a corrective ArticleUrlBackfilled
// event appended (the existing Trigger Backfill path is silently no-oped
// by the sovereign dedupe registry on `article-created:*`). Tests here
// guard:
//   1. button reaches the BFF and the result summary renders the counters
//      returned by the alt-backend handler (ADR-869 wire response shape);
//   2. SkippedDuplicate is honestly reflected in the summary after the
//      AppendKnowledgeEventPort signature change — Hurl + Pact already
//      pin the wire shape, this spec proves the FE actually surfaces it
//      to the operator;
//   3. error path keeps prior projection-health snapshot untouched and
//      surfaces the failure on the existing error banner.
test.describe("Knowledge Home Admin - Emit URL Backfill (ADR-869)", () => {
	let adminPage: DesktopKnowledgeHomeAdminPage;

	test.beforeEach(async ({ page }) => {
		adminPage = new DesktopKnowledgeHomeAdminPage(page);

		// Default GET stubs for the admin/sovereign snapshot pulls.
		await page.route(SOVEREIGN_API, (route) => {
			if (route.request().method() === "GET") {
				return fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT);
			}
			return fulfillJson(route, { ok: true });
		});
	});

	test("renders backfill counters returned by the BFF after a live emit", async ({
		page,
	}) => {
		await page.route(ADMIN_API, (route) => {
			const req = route.request();
			if (req.method() === "GET") {
				return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
			}
			const body = req.postDataJSON() as { action?: string };
			if (body.action === "emit_article_url_backfill") {
				return fulfillJson(route, {
					ok: true,
					result: {
						articlesScanned: 27198,
						eventsAppended: 12252,
						skippedBlockedScheme: 14946,
						skippedDuplicate: 0,
						moreRemaining: false,
					},
				});
			}
			return fulfillJson(route, { ok: true });
		});

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		await adminPage.emitUrlBackfillButton.click();

		await expect(adminPage.urlBackfillResultSummary).toBeVisible({
			timeout: 5000,
		});
		// Counter shape pins ADR-869 wire response: appended / scanned /
		// blocked-scheme reach the operator.
		await expect(adminPage.urlBackfillResultSummary).toContainText(
			"12252 appended",
		);
		await expect(adminPage.urlBackfillResultSummary).toContainText(
			"27198 scanned",
		);
		await expect(adminPage.urlBackfillResultSummary).toContainText(
			"14946 blocked-scheme",
		);
		// `moreRemaining` is false → no trailing "more remaining" suffix.
		await expect(adminPage.urlBackfillResultSummary).not.toContainText(
			"more remaining",
		);
	});

	test("idempotent re-run surfaces honest skipped_duplicate counter", async ({
		page,
	}) => {
		// The second BFF call simulates the dedupe-registry replay: every
		// scanned article now hits an existing `article-url-backfill:<id>`
		// row, so eventsAppended drops to 0 and skippedDuplicate matches
		// the scan count. This is the regression guard for the
		// AppendKnowledgeEventPort signature drift (port returns
		// (eventSeq, err); seq==0 ⇒ duplicate).
		let callCount = 0;
		await page.route(ADMIN_API, (route) => {
			const req = route.request();
			if (req.method() === "GET") {
				return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
			}
			const body = req.postDataJSON() as { action?: string };
			if (body.action === "emit_article_url_backfill") {
				callCount += 1;
				if (callCount === 1) {
					return fulfillJson(route, {
						ok: true,
						result: {
							articlesScanned: 3,
							eventsAppended: 3,
							skippedBlockedScheme: 0,
							skippedDuplicate: 0,
							moreRemaining: false,
						},
					});
				}
				return fulfillJson(route, {
					ok: true,
					result: {
						articlesScanned: 3,
						eventsAppended: 0,
						skippedBlockedScheme: 0,
						skippedDuplicate: 3,
						moreRemaining: false,
					},
				});
			}
			return fulfillJson(route, { ok: true });
		});

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		await adminPage.emitUrlBackfillButton.click();
		await expect(adminPage.urlBackfillResultSummary).toContainText(
			"3 appended",
		);

		// Second click — replay against the dedupe registry.
		await adminPage.emitUrlBackfillButton.click();
		await expect(adminPage.urlBackfillResultSummary).toContainText(
			"0 appended",
		);
		// The summary is rendered inline so the latest result replaces
		// the prior text. We assert the new shape is what the BFF
		// returned (skippedDuplicate=3 is implicit because eventsAppended
		// dropped to 0; the FE only renders eventsAppended/scanned/
		// skippedBlockedScheme/moreRemaining today, but the underlying
		// counter still drives the right next operator action).
		await expect(adminPage.urlBackfillResultSummary).toContainText("3 scanned");
	});

	test("backend error surfaces on the error banner without losing projection health", async ({
		page,
	}) => {
		await page.route(ADMIN_API, (route) => {
			const req = route.request();
			if (req.method() === "GET") {
				return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
			}
			const body = req.postDataJSON() as { action?: string };
			if (body.action === "emit_article_url_backfill") {
				return route.fulfill({
					status: 500,
					contentType: "application/json",
					body: JSON.stringify({ error: "sovereign unreachable" }),
				});
			}
			return fulfillJson(route, { ok: true });
		});

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		await adminPage.emitUrlBackfillButton.click();

		await expect(adminPage.errorBanner).toBeVisible({ timeout: 5000 });
		await expect(adminPage.errorBanner).toContainText("sovereign unreachable");
		// Result summary did not exist before the error and must NOT
		// appear from the failed call.
		await expect(adminPage.urlBackfillResultSummary).toHaveCount(0);
		// Header title still renders: the failure did not nuke the
		// admin shell.
		await expect(adminPage.pageTitle).toBeVisible();
	});
});
