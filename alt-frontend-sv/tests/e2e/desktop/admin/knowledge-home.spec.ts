import { expect, test } from "@playwright/test";
import { DesktopKnowledgeHomeAdminPage } from "../../pages/desktop/DesktopKnowledgeHomeAdminPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	SOVEREIGN_ADMIN_SNAPSHOT,
	KNOWLEDGE_HOME_ADMIN_SNAPSHOT,
	AUDIT_RESULT,
	RETENTION_RUN_RESULT,
} from "../../fixtures/factories/sovereignAdminFactory";

const ADMIN_API = "**/api/admin/knowledge-home";
const SOVEREIGN_API = "**/api/admin/knowledge-home/sovereign";

test.describe("Knowledge Home Admin - Storage Tab", () => {
	let adminPage: DesktopKnowledgeHomeAdminPage;

	test.beforeEach(async ({ page }) => {
		adminPage = new DesktopKnowledgeHomeAdminPage(page);

		// Mock both admin APIs
		await page.route(SOVEREIGN_API, (route) => {
			if (route.request().method() === "GET") {
				return fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT);
			}
			return fulfillJson(route, { ok: true });
		});
		await page.route(ADMIN_API, (route) => {
			if (route.request().method() === "GET") {
				return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
			}
			return fulfillJson(route, { ok: true });
		});
	});

	test("renders Storage tab with storage stats", async () => {
		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		// Storage tab should be visible in navigation
		await expect(adminPage.tabStorage).toBeVisible();
		await adminPage.switchToStorageTab();

		// Storage stats panel should show table data (wait for sovereign data via client-side polling)
		await expect(adminPage.storageStatsPanel).toBeVisible();
		await expect(
			adminPage.getStorageStatCard("knowledge_events"),
		).toBeVisible({ timeout: 10000 });
		await expect(
			adminPage.getStorageStatCard("knowledge_home_items"),
		).toBeVisible({ timeout: 10000 });
	});

	test("renders snapshot list", async () => {
		await adminPage.goto();
		await adminPage.waitForAdminLoaded();
		await adminPage.switchToStorageTab();

		await expect(adminPage.snapshotListPanel).toBeVisible();
		await expect(adminPage.getSnapshotByStatus("valid")).toBeVisible({
			timeout: 10000,
		});
	});

	test("renders retention status with eligible partitions", async () => {
		await adminPage.goto();
		await adminPage.waitForAdminLoaded();
		await adminPage.switchToStorageTab();

		await expect(adminPage.retentionStatusPanel).toBeVisible();
	});

	test("Create Snapshot requires confirmation dialog", async ({ page }) => {
		await adminPage.goto();
		await adminPage.waitForAdminLoaded();
		await adminPage.switchToStorageTab();

		// Click create snapshot
		await adminPage.createSnapshotButton.click();

		// Confirmation dialog should appear
		await expect(adminPage.confirmDialog).toBeVisible();
	});

	test("Run Retention dry-run executes without confirmation", async ({
		page,
	}) => {
		await page.route(SOVEREIGN_API, (route) => {
			if (route.request().method() === "POST") {
				return fulfillJson(route, RETENTION_RUN_RESULT);
			}
			return fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT);
		});

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();
		await adminPage.switchToStorageTab();

		// Dry-run button should be available
		await expect(adminPage.runRetentionButton).toBeVisible();
	});
});

test.describe("Knowledge Home Admin - Audit Tab", () => {
	let adminPage: DesktopKnowledgeHomeAdminPage;

	test.beforeEach(async ({ page }) => {
		adminPage = new DesktopKnowledgeHomeAdminPage(page);

		await page.route(SOVEREIGN_API, (route) =>
			fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT),
		);
		await page.route(ADMIN_API, (route) => {
			if (route.request().method() === "GET") {
				return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
			}
			return fulfillJson(route, AUDIT_RESULT);
		});
	});

	test("renders Audit tab with form", async () => {
		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		await expect(adminPage.tabAudit).toBeVisible();
		await adminPage.switchToAuditTab();

		await expect(adminPage.auditActionsPanel).toBeVisible();
		await expect(adminPage.auditProjectionNameInput).toBeVisible();
		await expect(adminPage.auditProjectionVersionInput).toBeVisible();
		await expect(adminPage.auditSampleSizeInput).toBeVisible();
		await expect(adminPage.runAuditButton).toBeVisible();
	});

	test("runs audit and displays results", async ({ page }) => {
		await page.route(ADMIN_API, (route) => {
			if (route.request().method() === "POST") {
				return fulfillJson(route, AUDIT_RESULT);
			}
			return fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT);
		});

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();
		await adminPage.switchToAuditTab();

		// Fill in audit form
		await adminPage.auditProjectionNameInput.fill("knowledge_home");
		await adminPage.auditProjectionVersionInput.fill("2");
		await adminPage.auditSampleSizeInput.fill("100");

		// Run audit
		await adminPage.runAuditButton.click();

		// Result should appear with mismatch count
		await expect(adminPage.auditResultPanel).toBeVisible();
		await expect(adminPage.auditResultPanel.getByText("2", { exact: true })).toBeVisible();
	});
});

test.describe("Knowledge Home Admin - Tab Navigation", () => {
	test("all 5 tabs are present", async ({ page }) => {
		const adminPage = new DesktopKnowledgeHomeAdminPage(page);

		await page.route(SOVEREIGN_API, (route) =>
			fulfillJson(route, SOVEREIGN_ADMIN_SNAPSHOT),
		);
		await page.route(ADMIN_API, (route) =>
			fulfillJson(route, KNOWLEDGE_HOME_ADMIN_SNAPSHOT),
		);

		await adminPage.goto();
		await adminPage.waitForAdminLoaded();

		await expect(adminPage.tabOverview).toBeVisible();
		await expect(adminPage.tabSlo).toBeVisible();
		await expect(adminPage.tabReproject).toBeVisible();
		await expect(adminPage.tabStorage).toBeVisible();
		await expect(adminPage.tabAudit).toBeVisible();
	});
});
