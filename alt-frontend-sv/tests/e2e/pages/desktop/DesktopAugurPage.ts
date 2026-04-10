import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Augur (AI Q&A) page (/desktop/augur)
 */
export class DesktopAugurPage extends BasePage {
	// Masthead
	readonly pageTitle: Locator;

	// Thread container
	readonly threadContainer: Locator;

	// States
	readonly emptyState: Locator;
	readonly loadingIndicator: Locator;

	// Input area
	readonly chatInput: Locator;
	readonly sendButton: Locator;

	constructor(page: Page) {
		super(page);

		// Empty state title or thread heading
		this.pageTitle = page.getByText(/ask augur/i);

		// Thread container
		this.threadContainer = page.locator(".augur-thread");

		// States
		this.emptyState = page.locator(".augur-empty");
		this.loadingIndicator = page.locator(".augur-loading");

		// Input
		this.chatInput = page.getByRole("textbox");
		this.sendButton = page.getByRole("button", { name: /submit/i });
	}

	get url(): string {
		return "./augur";
	}

	/**
	 * Get all thread entries (Q&A pairs)
	 */
	getChatMessages(): Locator {
		return this.page.locator("[data-role]");
	}

	/**
	 * Get the last entry in the thread
	 */
	getLastMessage(): Locator {
		return this.getChatMessages().last();
	}

	/**
	 * Get user question entries only
	 */
	getUserMessages(): Locator {
		return this.page.locator('[data-role="user"]');
	}

	/**
	 * Get assistant answer entries only
	 */
	getAssistantMessages(): Locator {
		return this.page.locator('[data-role="assistant"]');
	}

	/**
	 * Send a message in the thread
	 */
	async sendMessage(message: string): Promise<void> {
		await this.chatInput.fill(message);
		await this.sendButton.click();
	}

	/**
	 * Wait for the AI to finish responding
	 */
	async waitForResponse(timeout = 30000): Promise<void> {
		await expect(this.loadingIndicator).not.toBeVisible({ timeout });
	}

	/**
	 * Get the text content of the last assistant entry
	 */
	async getLastAssistantMessage(): Promise<string> {
		const messages = this.getAssistantMessages();
		const lastMessage = messages.last();
		return (await lastMessage.textContent()) ?? "";
	}

	/**
	 * Check if the input is disabled (during loading)
	 */
	async isInputDisabled(): Promise<boolean> {
		const disabled = await this.chatInput.getAttribute("disabled");
		return disabled !== null;
	}

	/**
	 * Wait for empty state to appear (replaces welcome message)
	 */
	async waitForWelcomeMessage(): Promise<void> {
		await expect(this.emptyState).toBeVisible({ timeout: 10000 });
	}
}
