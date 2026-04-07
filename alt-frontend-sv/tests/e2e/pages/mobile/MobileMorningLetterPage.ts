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

	readonly chatToggle: Locator;

	constructor(page: Page) {
		super(page);

		this.pageHeader = page
			.getByRole("heading", { name: /morning letter/i })
			.first();
		this.chatToggle = page.getByRole("button", { name: /follow-up chat/i });
		this.chatInput = page.getByPlaceholder(
			/ask about the briefing|ask about today/i,
		);
		this.sendButton = page.getByLabel("Send");
		this.welcomeMessage = page.getByText(
			/follow-up questions|hello.*ask me about/i,
		);
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
	 * Wait for chat to be ready — opens the disclosure toggle if needed.
	 */
	async waitForChatReady(): Promise<void> {
		// Wait for page to load first
		await this.page
			.waitForLoadState("networkidle", { timeout: 15000 })
			.catch(() => {});

		// Mobile layout uses disclosure pattern — try to open it
		const toggle = this.chatToggle;
		try {
			await toggle.waitFor({ state: "visible", timeout: 5000 });
			const expanded = await toggle.getAttribute("aria-expanded");
			if (expanded !== "true") {
				await toggle.click();
			}
		} catch {
			// No toggle found — may be in desktop layout or legacy layout
		}

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
