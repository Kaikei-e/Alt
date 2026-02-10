import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop Augur (AI Chat) page (/desktop/augur)
 */
export class DesktopAugurPage extends BasePage {
	// Page header
	readonly pageTitle: Locator;

	// Chat container
	readonly chatContainer: Locator;

	// Messages
	readonly welcomeMessage: Locator;
	readonly thinkingIndicator: Locator;

	// Input area
	readonly chatInput: Locator;
	readonly sendButton: Locator;

	constructor(page: Page) {
		super(page);

		// Page elements
		this.pageTitle = page.getByRole("heading", { name: /ask augur/i });

		// Chat container
		this.chatContainer = page
			.locator(".flex.flex-col")
			.filter({ hasText: /augur/i });

		// Messages
		this.welcomeMessage = page.getByText(/hello! i'm augur/i);
		this.thinkingIndicator = page.getByText(/augur is (thinking|reasoning)/i);

		// Input - ChatInput component uses a textarea or input
		this.chatInput = page.getByRole("textbox");
		this.sendButton = page.getByRole("button", { name: /send/i });
	}

	get url(): string {
		return "./augur";
	}

	/**
	 * Get all chat messages
	 */
	getChatMessages(): Locator {
		// Each ChatMessage component renders with message text
		return this.page
			.locator('[class*="rounded-2xl"]')
			.filter({ hasText: /.+/ });
	}

	/**
	 * Get the last message in the chat
	 */
	getLastMessage(): Locator {
		return this.getChatMessages().last();
	}

	/**
	 * Get user messages only
	 */
	getUserMessages(): Locator {
		// User messages typically aligned right or have different styling
		return this.page.locator('[class*="bg-primary"]');
	}

	/**
	 * Get assistant messages only
	 */
	getAssistantMessages(): Locator {
		return this.page.locator('[class*="bg-muted"]');
	}

	/**
	 * Send a message in the chat
	 */
	async sendMessage(message: string): Promise<void> {
		await this.chatInput.fill(message);
		await this.sendButton.click();
	}

	/**
	 * Wait for the AI to finish responding
	 * Note: With fast mocked responses, the thinking indicator might not appear
	 */
	async waitForResponse(timeout = 30000): Promise<void> {
		// Wait for thinking indicator to disappear (or never appear if response is fast)
		await expect(this.thinkingIndicator).not.toBeVisible({ timeout });
	}

	/**
	 * Get the text content of the last assistant message
	 */
	async getLastAssistantMessage(): Promise<string> {
		const messages = this.getAssistantMessages();
		const lastMessage = messages.last();
		return (await lastMessage.textContent()) ?? "";
	}

	/**
	 * Check if the chat input is disabled (during loading)
	 */
	async isInputDisabled(): Promise<boolean> {
		const disabled = await this.chatInput.getAttribute("disabled");
		return disabled !== null;
	}

	/**
	 * Wait for welcome message to appear
	 */
	async waitForWelcomeMessage(): Promise<void> {
		await expect(this.welcomeMessage).toBeVisible({ timeout: 10000 });
	}
}
