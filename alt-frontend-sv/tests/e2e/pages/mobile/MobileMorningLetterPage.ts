import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Morning Letter page (/recap/morning-letter)
 */
export class MobileMorningLetterPage extends BasePage {
	readonly pageHeader: Locator;
	readonly chatInput: Locator;
	readonly sendButton: Locator;
	readonly welcomeMessage: Locator;
	readonly thinkingIndicator: Locator;
	readonly floatingMenu: Locator;
	readonly messageList: Locator;

	constructor(page: Page) {
		super(page);

		this.pageHeader = page
			.getByRole("heading", { name: /morning letter/i })
			.first();
		this.chatInput = page.getByPlaceholder(/ask about today/i);
		this.sendButton = page.getByLabel("Send");
		this.welcomeMessage = page.getByText(/hello.*ask me about/i);
		this.thinkingIndicator = page.getByText(/searching/i);
		this.floatingMenu = page.getByLabel("Open floating menu");
		this.messageList = page
			.locator('[role="log"]')
			.or(page.locator(".flex.flex-col.gap"));
	}

	get url(): string {
		return "./recap/morning-letter";
	}

	/**
	 * Wait for chat to be ready.
	 */
	async waitForChatReady(): Promise<void> {
		await expect(this.chatInput).toBeVisible({ timeout: 15000 });
	}

	/**
	 * Send a message in the chat.
	 */
	async sendMessage(text: string): Promise<void> {
		await this.chatInput.fill(text);
		await this.sendButton.click();
	}
}
