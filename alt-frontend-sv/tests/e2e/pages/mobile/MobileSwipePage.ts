import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Mobile Swipe Feed Page
 *
 * Encapsulates all interactions with the mobile swipe feed interface,
 * following Page Object Model best practices for maintainable E2E tests.
 */
export class MobileSwipePage extends BasePage {
	// Locators
	readonly swipeCard: Locator;
	readonly actionFooter: Locator;
	readonly articleButton: Locator;
	readonly summaryButton: Locator;
	readonly scrollArea: Locator;
	readonly aiSummarySection: Locator;
	readonly contentSection: Locator;
	readonly floatingMenu: Locator;
	readonly loadingOverlay: Locator;

	constructor(page: Page) {
		super(page);

		// Main elements
		this.swipeCard = page.getByTestId("swipe-card");
		this.actionFooter = page.getByTestId("action-footer");
		this.scrollArea = page.getByTestId("unified-scroll-area");

		// Buttons
		this.articleButton = page.getByRole("button", { name: /article/i });
		this.summaryButton = page.getByRole("button", { name: /summary/i });

		// Content sections
		this.aiSummarySection = page.getByTestId("ai-summary-section");
		this.contentSection = page.getByTestId("content-section");

		// UI components
		this.floatingMenu = page.getByTestId("floating-menu");
		this.loadingOverlay = page.getByTestId("swipe-loading-overlay");
	}

	get url(): string {
		return "feeds/swipe";
	}

	/**
	 * Get the current card title
	 */
	async getCardTitle(): Promise<string | null> {
		const heading = this.swipeCard.locator("h2");
		return heading.textContent();
	}

	/**
	 * Get card external link
	 */
	getExternalLink(): Locator {
		return this.swipeCard.locator('a[target="_blank"]');
	}

	/**
	 * Click the Article button to expand/collapse content
	 */
	async toggleArticleContent(): Promise<void> {
		await this.articleButton.click();
	}

	/**
	 * Click the Summary button to generate AI summary
	 */
	async requestAiSummary(): Promise<void> {
		await this.summaryButton.click();
	}

	/**
	 * Wait for AI summary to appear
	 */
	async waitForAiSummary(timeout = 30000): Promise<void> {
		await expect(this.aiSummarySection).toBeVisible({ timeout });
		// Wait for spinner to disappear
		await expect(
			this.aiSummarySection.locator(".animate-spin"),
		).not.toBeVisible({ timeout });
	}

	/**
	 * Wait for article content to load
	 */
	async waitForContentLoaded(timeout = 15000): Promise<void> {
		await expect(this.contentSection).toBeVisible({ timeout });
	}

	/**
	 * Perform a swipe left gesture (dismiss/skip)
	 */
	async swipeLeft(): Promise<void> {
		const box = await this.swipeCard.boundingBox();
		if (!box) throw new Error("Swipe card not found");

		const centerX = box.x + box.width / 2;
		const centerY = box.y + box.height / 2;

		await this.page.mouse.move(centerX, centerY);
		await this.page.mouse.down();
		await this.page.mouse.move(box.x - 200, centerY, { steps: 10 });
		await this.page.mouse.up();
	}

	/**
	 * Perform a swipe right gesture (mark as read/save)
	 */
	async swipeRight(): Promise<void> {
		const box = await this.swipeCard.boundingBox();
		if (!box) throw new Error("Swipe card not found");

		const centerX = box.x + box.width / 2;
		const centerY = box.y + box.height / 2;

		await this.page.mouse.move(centerX, centerY);
		await this.page.mouse.down();
		await this.page.mouse.move(box.x + box.width + 200, centerY, { steps: 10 });
		await this.page.mouse.up();
	}

	/**
	 * Wait for next card to appear after swipe
	 */
	async waitForNextCard(timeout = 5000): Promise<void> {
		await expect(this.swipeCard).toBeVisible({ timeout });
	}

	/**
	 * Check if card is visible
	 */
	async isCardVisible(): Promise<boolean> {
		return this.swipeCard.isVisible();
	}

	/**
	 * Check if action footer is visible
	 */
	async isFooterVisible(): Promise<boolean> {
		return this.actionFooter.isVisible();
	}

	/**
	 * Assert card has correct accessibility attributes
	 */
	async assertAccessibility(): Promise<void> {
		// Check aria-busy attribute
		const ariaBusy = await this.swipeCard.getAttribute("aria-busy");
		expect(ariaBusy).toBeDefined();

		// Check external link has security attributes
		const link = this.getExternalLink();
		await expect(link).toHaveAttribute("rel", "noopener noreferrer");
		await expect(link).toHaveAttribute("target", "_blank");
	}

	/**
	 * Scroll within the card content area
	 */
	async scrollContentDown(pixels = 200): Promise<void> {
		await this.scrollArea.evaluate((el, scrollAmount) => {
			el.scrollBy(0, scrollAmount);
		}, pixels);
	}

	/**
	 * Check if content is scrollable
	 */
	async isContentScrollable(): Promise<boolean> {
		return this.scrollArea.evaluate((el) => {
			return el.scrollHeight > el.clientHeight;
		});
	}

	/**
	 * Wait for page to be fully loaded
	 */
	async waitForPageReady(): Promise<void> {
		await expect(this.swipeCard).toBeVisible();
		await expect(this.actionFooter).toBeVisible();
	}

	/**
	 * Get the published date text if visible
	 */
	async getPublishedDate(): Promise<string | null> {
		const dateElement = this.swipeCard.locator("p").filter({
			hasText: /\d{4}|ago|yesterday|today/i,
		});
		if (await dateElement.isVisible()) {
			return dateElement.textContent();
		}
		return null;
	}
}
