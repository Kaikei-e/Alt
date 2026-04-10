import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Desktop MorningLetter page (/recap/morning-letter)
 * MorningLetter is a document-first briefing with editorial Q&A thread.
 */
export class DesktopMorningLetterPage extends BasePage {
	// Page header
	readonly pageTitle: Locator;

	// Chat container
	readonly chatContainer: Locator;

	// Messages
	readonly welcomeMessage: Locator;
	readonly thinkingIndicator: Locator;

	// Metadata display
	readonly timeWindowDisplay: Locator;
	readonly articlesScannedDisplay: Locator;

	// Input area
	readonly chatInput: Locator;
	readonly sendButton: Locator;

	constructor(page: Page) {
		super(page);

		// Page elements
		this.pageTitle = page
			.getByRole("heading", { name: /morning letter/i })
			.first();

		// Chat container
		this.chatContainer = page.locator(".letter-chat");

		// Messages - use partial match for dynamic content
		this.welcomeMessage = page.getByText(/follow-up questions about today/i);
		this.thinkingIndicator = page.getByText(
			/searching recent news|searching\.\.\./i,
		);

		// Metadata display
		this.timeWindowDisplay = page.getByText(/articles from/i);
		this.articlesScannedDisplay = page.getByText(/articles scanned/i);

		// Input - uses textarea
		this.chatInput = page.getByRole("textbox");
		this.sendButton = page.getByRole("button", { name: /send|submit/i });
	}

	get url(): string {
		return "./recap/morning-letter";
	}

	/**
	 * Get all chat messages (thread entries)
	 */
	getChatMessages(): Locator {
		return this.page.locator('[data-role="user"], [data-role="assistant"]');
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
		return this.page.locator('[data-role="user"]');
	}

	/**
	 * Get assistant messages only
	 */
	getAssistantMessages(): Locator {
		return this.page.locator('[data-role="assistant"]');
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
	 */
	async waitForResponse(timeout = 30000): Promise<void> {
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

	/**
	 * Get citations from the last response
	 */
	getCitations(): Locator {
		return this.page.locator(".entry-sources").getByRole("link");
	}
}
