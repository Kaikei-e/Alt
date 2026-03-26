import type { Locator, Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Knowledge Home Admin Console.
 */
export class DesktopKnowledgeHomeAdminPage extends BasePage {
	// Page header
	readonly pageTitle: Locator;

	// Tab navigation
	readonly tabOverview: Locator;
	readonly tabSlo: Locator;
	readonly tabReproject: Locator;
	readonly tabStorage: Locator;
	readonly tabAudit: Locator;

	// Storage tab elements
	readonly storageStatsPanel: Locator;
	readonly snapshotListPanel: Locator;
	readonly retentionStatusPanel: Locator;
	readonly createSnapshotButton: Locator;
	readonly runRetentionButton: Locator;
	readonly confirmDialog: Locator;
	readonly confirmDialogConfirmButton: Locator;

	// Audit tab elements
	readonly auditActionsPanel: Locator;
	readonly auditResultPanel: Locator;
	readonly auditProjectionNameInput: Locator;
	readonly auditProjectionVersionInput: Locator;
	readonly auditSampleSizeInput: Locator;
	readonly runAuditButton: Locator;

	constructor(page: Page) {
		super(page);

		this.pageTitle = page.getByRole("heading", {
			name: "Knowledge Home Operations",
		});

		// Tabs
		this.tabOverview = page.getByRole("button", { name: "Overview" });
		this.tabSlo = page.getByRole("button", { name: "SLO" });
		this.tabReproject = page.getByRole("button", { name: "Reproject" });
		this.tabStorage = page.getByRole("button", { name: "Storage" });
		this.tabAudit = page.getByRole("button", { name: "Audit" });

		// Storage tab
		this.storageStatsPanel = page.getByTestId("storage-stats-panel");
		this.snapshotListPanel = page.getByTestId("snapshot-list-panel");
		this.retentionStatusPanel = page.getByTestId("retention-status-panel");
		this.createSnapshotButton = page.getByRole("button", {
			name: "Create Snapshot",
		});
		this.runRetentionButton = page.getByRole("button", {
			name: "Run Retention (Dry Run)",
		});
		this.confirmDialog = page.getByTestId("confirm-action-dialog");
		this.confirmDialogConfirmButton = page.getByTestId(
			"confirm-action-confirm",
		);

		// Audit tab
		this.auditActionsPanel = page.getByTestId("audit-actions-panel");
		this.auditResultPanel = page.getByTestId("audit-result-panel");
		this.auditProjectionNameInput = page.getByLabel("Projection Name");
		this.auditProjectionVersionInput = page.getByLabel("Projection Version");
		this.auditSampleSizeInput = page.getByLabel("Sample Size");
		this.runAuditButton = page.getByRole("button", { name: "Run Audit" });
	}

	get url(): string {
		return "/admin/knowledge-home";
	}

	async switchToStorageTab(): Promise<void> {
		await this.tabStorage.click();
	}

	async switchToAuditTab(): Promise<void> {
		await this.tabAudit.click();
	}

	async waitForAdminLoaded(): Promise<void> {
		await this.pageTitle.waitFor({ state: "visible", timeout: 10000 });
	}

	/** Get a storage stat card by table name text */
	getStorageStatCard(tableName: string): Locator {
		return this.storageStatsPanel.getByRole("cell", { name: tableName });
	}

	/** Get a snapshot row by status badge */
	getSnapshotByStatus(status: string): Locator {
		return this.snapshotListPanel.locator("span").filter({ hasText: status });
	}
}
