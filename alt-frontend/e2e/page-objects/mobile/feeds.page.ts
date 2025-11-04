import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Mobile Feeds Page Object
 * Represents the /mobile/feeds page
 */
export class MobileFeedsPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly feedsList: Locator;
  readonly floatingMenuButton: Locator;
  readonly floatingMenu: Locator;
  readonly floatingMenuItems: Locator;
  readonly addFeedButton: Locator;
  readonly searchButton: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", { name: /feeds/i });
    this.feedsList = page.getByRole("list").filter({
      has: page.getByRole("article").or(page.getByRole("listitem")),
    });
    this.floatingMenuButton = page.getByRole("button", {
      name: /menu|options|more/i,
    });
    this.floatingMenu = page.getByRole("menu").or(page.locator('[data-testid="floating-menu"]'));
    this.floatingMenuItems = this.floatingMenu.getByRole("menuitem");
    this.addFeedButton = page.getByRole("button", { name: /add|new feed/i });
    this.searchButton = page.getByRole("button", { name: /search/i });
    this.loadingIndicator = page.getByRole("status", { name: /loading/i });
    this.emptyState = page.getByText(/no feeds|empty/i);
    this.errorMessage = page.getByRole("alert");
  }

  /**
   * Navigate to mobile feeds page
   */
  async goto(): Promise<void> {
    await this.page.goto("/mobile/feeds");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();

    // Wait for feeds list or empty state
    try {
      await expect(this.feedsList).toBeVisible({ timeout: 5000 });
    } catch {
      try {
        await expect(this.emptyState).toBeVisible({ timeout: 2000 });
      } catch {
        // Neither visible
      }
    }

    await this.waitForLoadingToComplete();
  }

  /**
   * Get visible feed count
   */
  async getVisibleFeedCount(): Promise<number> {
    try {
      return await this.feedsList.getByRole("article").count();
    } catch {
      return 0;
    }
  }

  /**
   * Get first feed
   */
  async getFirstFeed(): Promise<Locator> {
    return this.feedsList.getByRole("article").first();
  }

  /**
   * Scroll to bottom (for infinite scroll)
   */
  async scrollToBottom(): Promise<void> {
    await super.scrollToBottom();
    await this.wait(500); // Wait for scroll to settle
  }

  /**
   * Swipe to mark as read
   */
  async swipeToMarkAsRead(feedElement: Locator): Promise<void> {
    const box = await feedElement.boundingBox();

    if (box) {
      // Swipe left
      await this.page.mouse.move(box.x + box.width - 10, box.y + box.height / 2);
      await this.page.mouse.down();
      await this.page.mouse.move(box.x + 10, box.y + box.height / 2);
      await this.page.mouse.up();

      await this.wait(300);
    }
  }

  /**
   * Open floating menu
   */
  async openFloatingMenu(): Promise<void> {
    await this.floatingMenuButton.click();
    await expect(this.floatingMenu).toBeVisible();
  }

  /**
   * Close floating menu
   */
  async closeFloatingMenu(): Promise<void> {
    // Click outside or press escape
    await this.page.keyboard.press("Escape");
    await expect(this.floatingMenu).not.toBeVisible();
  }

  /**
   * Select menu item
   */
  async selectMenuItem(itemName: string): Promise<void> {
    await this.openFloatingMenu();

    const menuItem = this.floatingMenuItems.filter({ hasText: itemName });
    await menuItem.click();
  }

  /**
   * Navigate to add feed
   */
  async goToAddFeed(): Promise<void> {
    if ((await this.addFeedButton.count()) > 0) {
      await this.addFeedButton.click();
    } else {
      await this.selectMenuItem("Add Feed");
    }

    await this.page.waitForURL(/\/mobile\/feeds\/register/);
  }

  /**
   * Navigate to search
   */
  async goToSearch(): Promise<void> {
    if ((await this.searchButton.count()) > 0) {
      await this.searchButton.click();
    } else {
      await this.selectMenuItem("Search");
    }

    await this.page.waitForURL(/\/mobile\/feeds\/search/);
  }

  /**
   * Navigate to favorites
   */
  async goToFavorites(): Promise<void> {
    await this.selectMenuItem("Favorites");
    await this.page.waitForURL(/\/mobile\/feeds\/favorites/);
  }

  /**
   * Navigate to stats
   */
  async goToStats(): Promise<void> {
    await this.selectMenuItem("Stats");
    await this.page.waitForURL(/\/mobile\/feeds\/stats/);
  }

  /**
   * Check if empty state is shown
   */
  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if error is shown
   */
  async hasError(): Promise<boolean> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Pull to refresh
   */
  async pullToRefresh(): Promise<void> {
    // Simulate pull-to-refresh gesture
    await this.scrollToTop();
    await this.page.mouse.move(200, 100);
    await this.page.mouse.down();
    await this.page.mouse.move(200, 300);
    await this.page.mouse.up();

    await this.waitForLoadingToComplete();
  }

  /**
   * Tap on feed
   */
  async tapFeed(index: number): Promise<void> {
    const feeds = this.feedsList.getByRole("article");
    await feeds.nth(index).click();
  }

  /**
   * Long press on feed
   */
  async longPressFeed(index: number): Promise<void> {
    const feeds = this.feedsList.getByRole("article");
    const feed = feeds.nth(index);

    await feed.click({ delay: 1000 }); // Long press
  }
}
